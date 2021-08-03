package main

import (
	"net/http"
	"os"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

type recorderFunc func(*cli.Context, v0api.FullNode, chan error)

var (
	log       = logging.Logger("lotus-monitor")
	recorders = []recorderFunc{
		actorRecorder,
		minerRecorder,
	}
)

func main() {
	app := &cli.App{
		Name:        "lotus-monitor",
		Usage:       "monitor actor addresses",
		Version:     build.UserVersion(),
		Description: "Export actor attributes as prometheus metrics",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "listen",
				Value: "0.0.0.0:0",
				EnvVars: []string{
					"LOTUS_MONITOR_LISTEN",
				},
			},
			&cli.DurationFlag{
				Name:  "poll",
				Value: time.Minute,
				EnvVars: []string{
					"LOTUS_MONITOR_POLL_FREQUENCY",
				},
			},
			&cli.StringSliceFlag{
				Name:  "actors",
				Usage: "Actor or Wallet addresses to monitor",
				EnvVars: []string{
					"LOTUS_MONITOR_ACTORS",
				},
			},
			&cli.StringSliceFlag{
				Name:  "miners",
				Usage: "Miner addresses to monitor",
				EnvVars: []string{
					"LOTUS_MONITOR_MINERS",
				},
			},
		},
		Action: func(cctx *cli.Context) error {
			api, closer, err := cliutil.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			if err := view.Register(balanceView, nonceView); err != nil {
				return err
			}

			pe, err := prometheus.NewExporter(prometheus.Options{
				Namespace: "lotusmonitor",
			})
			if err != nil {
				log.Fatalw("failed to create the Prometheus stats exporter", "err", err)
			}

			go func() {
				mux := http.NewServeMux()
				mux.Handle("/metrics", pe)
				if err := http.ListenAndServe(cctx.String("listen"), mux); err != nil {
					log.Fatalw("failed to run endpoint", "err", err)
				}
			}()

			recordErrs := make(chan error)
			for _, f := range recorders {
				go f(cctx, api, recordErrs)
			}
			errorRecorder(cctx, recordErrs)
			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%+v", err)
	}
}