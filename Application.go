package pgo

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "reflect"
    "runtime"
    "strings"
    "sync"
)

// app initialization steps:
// 1. import pgo: pgo.init()
// 2. customize app: optional
// 3. pgo.Run(): serve
//
// configuration:
// {
//     "name": "app-name",
//     "GOMAXPROCS": 4,
//     "runtimePath": "@app/runtime",
//     "publicPath": "@app/public",
//     "viewPath": "@viewPath",
//     "server": {},
//     "components": {}
// }
type Application struct {
    mode        int
    env         string
    name        string
    basePath    string
    runtimePath string
    publicPath  string
    viewPath    string
    config      *Config
    container   *Container
    server      *Server
    components  map[string]interface{}
    lock        sync.RWMutex
    router      *Router
    log         *Dispatcher
    status      *Status
    i18n        *I18n
    view        *View
}

func (app *Application) Construct() {
    exeBase := filepath.Base(os.Args[0])
    exeExt := filepath.Ext(os.Args[0])
    exeDir := filepath.Dir(os.Args[0])

    app.env = DefaultEnv
    app.mode = ModeWeb
    app.name = strings.TrimSuffix(exeBase, exeExt)
    app.basePath, _ = filepath.Abs(filepath.Join(exeDir, ".."))
    app.config = &Config{}
    app.container = &Container{}
    app.server = &Server{}
    app.components = make(map[string]interface{})
}

func (app *Application) Init() {
    env := flag.String("env", "", "set running env, eg. --env prod")
    cmd := flag.String("cmd", "", "set running cmd, eg. --cmd /foo/bar")
    base := flag.String("base", "", "set base path, eg. --base /base/path")
    flag.Parse()

    // overwrite running env
    if len(*env) > 0 {
        app.env = *env
    } else {
        env := os.Getenv("env")
        if len(env) > 0 {
            app.env = env
        }
    }

    // overwrite running mode
    if len(*cmd) > 0 {
        app.mode = ModeCmd
    }

    // overwrite base path
    if len(*base) > 0 {
        app.basePath, _ = filepath.Abs(*base)
    }

    // initialize config object
    ConstructAndInit(app.config, nil)

    // initialize container object
    ConstructAndInit(app.container, nil)

    // initialize server object
    svrConf, _ := app.config.Get("app.server").(map[string]interface{})
    ConstructAndInit(app.server, svrConf)

    // set basic path alias
    type dummy struct{}
    pkgPath := reflect.TypeOf(dummy{}).PkgPath()
    SetAlias("@app", app.basePath)
    SetAlias("@pgo", strings.TrimPrefix(pkgPath, VendorPrefix))

    // overwrite app name
    if name := app.config.GetString("app.name", ""); len(name) > 0 {
        app.name = name
    }

    // overwrite GOMAXPROCS
    if n := app.config.GetInt("app.GOMAXPROCS", 0); n > 0 {
        runtime.GOMAXPROCS(n)
    }

    // set runtime path
    runtimePath := app.config.GetString("app.runtimePath", "@app/runtime")
    app.runtimePath, _ = filepath.Abs(GetAlias(runtimePath))
    SetAlias("@runtime", app.runtimePath)

    // set public path
    publicPath := app.config.GetString("app.publicPath", "@app/public")
    app.publicPath, _ = filepath.Abs(GetAlias(publicPath))
    SetAlias("@public", app.publicPath)

    // set view path
    viewPath := app.config.GetString("app.viewPath", "@app/view")
    app.viewPath, _ = filepath.Abs(GetAlias(viewPath))
    SetAlias("@view", app.viewPath)

    // set core components
    for id, class := range app.coreComponents() {
        key := fmt.Sprintf("app.components.%s.class", id)
        app.config.Set(key, class)
    }

    // create runtime directory if not exists
    if _, e := os.Stat(app.runtimePath); os.IsNotExist(e) {
        if e := os.MkdirAll(app.runtimePath, 0755); e != nil {
            panic(fmt.Sprintf("failed to create %s, %s", app.runtimePath, e))
        }
    }
}

func (app *Application) GetMode() int {
    return app.mode
}

func (app *Application) GetEnv() string {
    return app.env
}

func (app *Application) GetName() string {
    return app.name
}

func (app *Application) GetBasePath() string {
    return app.basePath
}

func (app *Application) GetRuntimePath() string {
    return app.runtimePath
}

func (app *Application) GetPublicPath() string {
    return app.publicPath
}

func (app *Application) GetViewPath() string {
    return app.viewPath
}

func (app *Application) GetConfig() *Config {
    return app.config
}

func (app *Application) GetContainer() *Container {
    return app.container
}

func (app *Application) GetServer() *Server {
    return app.server
}

func (app *Application) GetRouter() *Router {
    if app.router == nil {
        app.router = app.Get("router").(*Router)
    }

    return app.router
}

func (app *Application) GetLog() *Dispatcher {
    if app.log == nil {
        app.log = app.Get("log").(*Dispatcher)
    }

    return app.log
}

func (app *Application) GetStatus() *Status {
    if app.status == nil {
        app.status = app.Get("status").(*Status)
    }

    return app.status
}

func (app *Application) GetI18n() *I18n {
    if app.i18n == nil {
        app.i18n = app.Get("i18n").(*I18n)
    }

    return app.i18n
}

func (app *Application) GetView() *View {
    if app.view == nil {
        app.view = app.Get("view").(*View)
    }

    return app.view
}

func (app *Application) Get(id string) interface{} {
    if _, ok := app.components[id]; !ok {
        app.loadComponent(id)
    }

    app.lock.RLock()
    defer app.lock.RUnlock()

    return app.components[id]
}

func (app *Application) loadComponent(id string) {
    app.lock.Lock()
    defer app.lock.Unlock()

    // avoid repeated loading
    if _, ok := app.components[id]; ok {
        return
    }

    conf := app.config.Get("app.components." + id)
    if conf == nil {
        panic("component not found: " + id)
    }

    app.components[id] = CreateObject(conf)
}

func (app *Application) coreComponents() map[string]string {
    return map[string]string{
        "router": "@pgo/Router",
        "log":    "@pgo/Dispatcher",
        "status": "@pgo/Status",
        "i18n":   "@pgo/I18n",
        "view":   "@pgo/View",

        "http": "@pgo/Client/Http/Client",
    }
}
