package apiserver

import (
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/config"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/apiserver/options"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/app"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

const commandDesc = `The IAM API server validates and configures data
for the api objects which include users, policies, secrets, and
others. The API Server services REST operations to do the api objects management.

Find more iam-apiserver information at:
    https://github.com/marmotedu/iam/blob/master/docs/guide/en-US/cmd/iam-apiserver.md`

func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp(basename, "iam-apiserver", app.WithOptions(opts), app.WithDescription(commandDesc), app.WithDefaultValidArgs(), app.WithRunFunc(run(opts)))
	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {

		log.Init(opts.Log)
		defer log.Flush()

		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}
		return Run(cfg)

	}

}
