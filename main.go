package main

import (
	"bytes"
	"io"
	"io/ioutil"

	"v2ray.com/core/common"
	"v2ray.com/core/infra/conf/serial"

	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"v2ray.com/core"
	"v2ray.com/core/common/platform"
	v2filesystem "v2ray.com/core/common/platform/filesystem"
)

const (
	v2Assert    = "v2ray.location.asset"
	assetperfix = "/dev/libv2rayfs0/asset"
)

var (
	configFile = flag.String("config", "", "Config file for V2Ray.")
	version    = flag.Bool("version", false, "Show current version.")
	test       = flag.Bool("test", false, "Test config file only, without launching V2Ray server.")
	format     = flag.String("format", "json", "Format of input file.")
	MAINVER    = "0.0.0-src"
)

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir()
}

func getConfigFilePath() string {
	if len(*configFile) > 0 {
		return *configFile
	}

	if workingDir, err := os.Getwd(); err == nil {
		configFile := filepath.Join(workingDir, "config.json")
		if fileExists(configFile) {
			return configFile
		}
	}

	if configFile := platform.GetConfigurationPath(); fileExists(configFile) {
		return configFile
	}

	return ""
}

func GetConfigFormat() string {
	switch strings.ToLower(*format) {
	case "pb", "protobuf":
		return "protobuf"
	default:
		return "json"
	}
}

func startV2Ray() (core.Server, error) {
	configFile := getConfigFilePath()
	configInput, err := LoadConfig(configFile)
	if err != nil {
		return nil, newError("failed to load config: ", configFile).Base(err)
	}
	defer configInput.Close()

	config, err := core.LoadConfig(GetConfigFormat(), configFile, configInput)
	if err != nil {
		return nil, newError("failed to read config file: ", configFile).Base(err)
	}

	server, err := core.New(config)
	if err != nil {
		return nil, newError("failed to create server").Base(err)
	}

	return server, nil
}

func printVersion() {
	fmt.Printf("gray [%s] Yet another distribution of v2ray (v2ray-core: %s)\n", MAINVER, core.Version())
}

func main() {
	flag.Parse()

	printVersion()

	if *version {
		return
	}

	server, err := startV2Ray()
	if err != nil {
		fmt.Println(err.Error())
		// Configuration error. Exit with a special value to prevent systemd from restarting.
		os.Exit(23)
	}

	if *test {
		fmt.Println("Configuration OK.")
		os.Exit(0)
	}

	if err := server.Start(); err != nil {
		fmt.Println("Failed to start", err)
		os.Exit(-1)
	}
	defer server.Close()

	// Explicitly triggering GC to remove garbage from config loading.
	runtime.GC()

	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
		<-osSignals
	}
}

func init() {
	common.Must(core.RegisterConfigLoader(&core.ConfigFormat{
		Name:      "JSON",
		Extension: []string{"json"},
		Loader:    serial.LoadJSONConfig,
	}))

	os.Setenv(v2Assert, assetperfix)
	//Now we handle read
	v2filesystem.NewFileReader = func(path string) (io.ReadCloser, error) {
		if strings.HasPrefix(path, assetperfix) {
			p := path[len(assetperfix)+1:]
			bin := MustAsset(p)
			return ioutil.NopCloser(bytes.NewReader(bin)), nil
		}
		return os.Open(path)
	}
}
