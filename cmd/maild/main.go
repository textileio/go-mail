package maild

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/textileio/go-mail"
	"github.com/textileio/go-mail/api/common"
	"github.com/textileio/go-mail/cmd"
	dbc "github.com/textileio/go-threads/api/client"
	"github.com/textileio/go-threads/core/did"
	nc "github.com/textileio/go-threads/net/api/client"
	"github.com/textileio/go-threads/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const daemonName = "maild"

var (
	log = logging.Logger(daemonName)

	config = &cmd.Config{
		Viper: viper.New(),
		Dir:   "." + daemonName,
		Name:  "config",
		Flags: map[string]cmd.Flag{
			"debug": {
				Key:      "log.debug",
				DefValue: false,
			},
			"logFile": {
				Key:      "log.file",
				DefValue: "",
			},

			// Addresses
			"addrApi": {
				Key:      "addr.api",
				DefValue: "127.0.0.1:5000",
			},
			"addrApiProxy": {
				Key:      "addr.api_proxy",
				DefValue: "127.0.0.1:5050",
			},

			// Threads
			"threadsAddr": {
				Key:      "threads.addr",
				DefValue: "127.0.0.1:4000",
			},
		},
	}
)

func init() {
	cobra.OnInitialize(cmd.InitConfig(config))
	cmd.InitConfigCmd(rootCmd, config.Viper, config.Dir)
	rootCmd.PersistentFlags().StringVar(
		&config.File,
		"config",
		"",
		"Config file (default ${HOME}/"+config.Dir+"/"+config.Name+".yml)")

	rootCmd.PersistentFlags().BoolP(
		"debug",
		"d",
		config.Flags["debug"].DefValue.(bool),
		"Enable debug logging")
	rootCmd.PersistentFlags().String(
		"logFile",
		config.Flags["logFile"].DefValue.(string),
		"Write logs to file")

	// Addresses
	rootCmd.PersistentFlags().String(
		"addrApi",
		config.Flags["addrApi"].DefValue.(string),
		"API listen address")

	// Threads
	rootCmd.PersistentFlags().String(
		"threadsAddr",
		config.Flags["threadsAddr"].DefValue.(string),
		"Threads API address")
}

func main() {
	cmd.ErrCheck(rootCmd.Execute())
}

var rootCmd = &cobra.Command{
	Use:   daemonName,
	Short: "Mail Daemon",
	Long:  "The Mail Daemon.",
	PersistentPreRun: func(c *cobra.Command, args []string) {
		config.Viper.SetConfigType("yaml")
		cmd.ExpandConfigVars(config.Viper, config.Flags)

		if config.Viper.GetBool("log.debug") {
			err := util.SetLogLevels(map[string]logging.LogLevel{
				daemonName: logging.LevelDebug,
				"mail":     logging.LevelDebug,
			})
			cmd.ErrCheck(err)
		}
	},
	Run: func(c *cobra.Command, args []string) {
		settings, err := json.MarshalIndent(config.Viper.AllSettings(), "", "  ")
		cmd.ErrCheck(err)
		log.Debugf("loaded config: %s", string(settings))

		logFile := config.Viper.GetString("log.file")
		if logFile != "" {
			err = cmd.SetupDefaultLoggingConfig(logFile)
			cmd.ErrCheck(err)
		}

		addrApi := config.Viper.GetString("addr.api")
		addrApiProxy := config.Viper.GetString("addr.api_proxy")

		threadsApi := config.Viper.GetString("threads.addr")

		net, err := nc.NewClient(threadsApi, getClientRPCOpts(threadsApi)...)
		cmd.ErrCheck(err)
		db, err := dbc.NewClient(threadsApi, getClientRPCOpts(threadsApi)...)
		cmd.ErrCheck(err)

		lib, err := mail.NewMail(db, net)
		cmd.ErrCheck(err)

		server, proxy, err := common.GetServerAndProxy(lib, addrApi, addrApiProxy)
		cmd.ErrCheck(err)

		cmd.ErrCheck(err)

		fmt.Println("Welcome to Mail!")

		cmd.HandleInterrupt(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = proxy.Shutdown(ctx)
			cmd.LogErr(err)
			log.Info("proxy was shutdown")

			stopped := make(chan struct{})
			go func() {
				server.GracefulStop()
				close(stopped)
			}()
			timer := time.NewTimer(10 * time.Second)
			select {
			case <-timer.C:
				server.Stop()
			case <-stopped:
				timer.Stop()
			}

			err = lib.Close()
			cmd.LogErr(err)
			log.Info("bucket lib was shutdown")

			err = db.Close()
			cmd.LogErr(err)
			log.Info("db client was shutdown")

			err = net.Close()
			cmd.LogErr(err)
			log.Info("net client was shutdown")
		})
	},
}

func getClientRPCOpts(target string) (opts []grpc.DialOption) {
	creds := did.RPCCredentials{}
	if strings.Contains(target, "443") {
		tcreds := credentials.NewTLS(&tls.Config{})
		opts = append(opts, grpc.WithTransportCredentials(tcreds))
		creds.Secure = true
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	opts = append(opts, grpc.WithPerRPCCredentials(creds))
	return opts
}
