package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/doomsday-project/doomsday/client/doomsday"
	"github.com/doomsday-project/doomsday/version"
	"github.com/starkandwayne/goutils/ansi"
)

func registerCommands(app *kingpin.Application) {
	serverCom := app.Command("server", "Start the doomsday server")
	cmdIndex["server"] = &serverCmd{
		ManifestPath: serverCom.Flag("manifest", "The path to the server manifest").
			Short('m').
			Default("ddayconfig.yml").String(),
	}

	targetCom := app.Command("target", "Manage targeted doomsday servers")
	cmdIndex["target"] = &targetCmd{
		Name:    targetCom.Arg("name", "The name of the target").String(),
		Address: targetCom.Arg("address", "The address to set for this target").String(),
		SkipVerify: targetCom.Flag("insecure", "Skip TLS cert validation for this backend").
			Short('k').Bool(),
		Delete: targetCom.Flag("delete", "Forget about the doomsday target with "+
			"the given name. Delete will not fail if the target does not exist").Short('d').Bool(),
	}

	_ = app.Command("targets", "Print out configured targets")
	cmdIndex["targets"] = &targetsCmd{}

	loginCom := app.Command("login", "Auth to the doomsday server").Alias("auth")
	cmdIndex["login"] = &loginCmd{
		Username: loginCom.Flag("username", "The username to log in as").
			Short('u').String(),
		Password: loginCom.Flag("password", "The password to log in with").
			Short('p').String(),
	}
	cmdIndex["auth"] = cmdIndex["login"]

	listCom := app.Command("list", "List the contents of the server cache")
	cmdIndex["list"] = &listCmd{
		Beyond: listCom.Flag("beyond", "Restrict to certs that expire in longer than the given duration").
			Short('b').PlaceHolder("1y2d3h4m").String(),
		Within: listCom.Flag("within", "Restrict to certs that expire in less than the given duration").
			Short('w').PlaceHolder("1y2d3h4m").String(),
	}

	_ = app.Command("dashboard", "See your impending doom").Alias("dash")
	cmdIndex["dashboard"] = &dashboardCmd{}
	cmdIndex["dash"] = cmdIndex["dashboard"]

	_ = app.Command("scheduler", "View the current state of the doomsday scheduler").Alias("sched").Hidden()
	cmdIndex["scheduler"] = &schedulerCmd{}
	cmdIndex["sched"] = cmdIndex["scheduler"]

	_ = app.Command("refresh", "Refresh the servers cache")
	cmdIndex["refresh"] = &refreshCmd{}

	_ = app.Command("info", "Get info about the currently targeted doomsday server")
	cmdIndex["info"] = &infoCmd{}
}

var app = kingpin.New("doomsday", "Cert expiration tracker")
var cliConf *CLIConfig
var target *CLITarget
var client *doomsday.Client

func main() {
	registerCommands(app)
	app.Version(version.Version)
	app.VersionFlag.Short('v')
	app.HelpFlag.Short('h')

	commandName := kingpin.MustParse(app.Parse(os.Args[1:]))
	cmd, found := cmdIndex[commandName]
	if !found {
		panic(fmt.Sprintf("Unregistered command %s", commandName))
	}

	if _, isServerCommand := cmd.(*serverCmd); !isServerCommand {
		var err error
		cliConf, err = loadConfig(*configPath)
		if err != nil {
			bailWith("Could not load CLI config from `%s': %s", *configPath, err)
		}

		target = cliConf.CurrentTarget()
	}

	switch cmd.(type) {
	case *serverCmd, *targetCmd, *targetsCmd:
	default:
		target = cliConf.CurrentTarget()
		if target == nil {
			bailWith("No doomsday server is currently targeted")
		}

		u, err := url.Parse(target.Address)
		if err != nil {
			bailWith("Could not parse target address as URL")
		}

		var traceWriter io.Writer
		if trace != nil && *trace {
			traceWriter = os.Stderr
		}

		client = &doomsday.Client{
			URL:   *u,
			Token: target.Token,
			Client: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: target.SkipVerify,
					},
				},
			},
			Trace: traceWriter,
		}
	}

	err := cmd.Run()
	if err != nil {
		if _, is401 := err.(*doomsday.ErrUnauthorized); is401 {
			err = fmt.Errorf("Not authenticated. Please log in with `doomsday login'")
		}
		bailWith(err.Error())
	}

	err = cliConf.saveConfig(*configPath)
	if err != nil {
		bailWith("Could not save config: %s", err)
	}
}

func bailWith(f string, a ...interface{}) {
	ansi.Fprintf(os.Stderr, fmt.Sprintf("@R{%s}\n", f), a...)
	os.Exit(1)
}
