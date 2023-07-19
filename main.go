package main

import (
	"context"
	"sync"

	"github.com/cilium/cilium/pkg/hive"
	"github.com/cilium/cilium/pkg/hive/cell"
	k8sClient "github.com/cilium/cilium/pkg/k8s/client"
	"github.com/cilium/cilium/pkg/logging"
	"github.com/mainred/manage-cilium-endpoint/pkg/watchers"
	"github.com/spf13/pflag"
)

type podWatcher struct {
}

var (
	podWatcherCell = cell.Module(
		"podwatcher",
		"pod watcher",

		cell.Invoke(
			registerOperatorHooks,
		),
	)
)

func main() {
	logging.SetLogLevelToDebug()
	h := hive.New(
		k8sClient.Cell,
		podWatcherCell,
	)

	h.RegisterFlags(pflag.CommandLine)

	pflag.Parse()
	h.PrintDotGraph()
	h.PrintObjects()
	h.Run()
}

func registerOperatorHooks(lc hive.Lifecycle, clientset k8sClient.Clientset, shutdowner hive.Shutdowner) {
	var wg sync.WaitGroup
	lc.Append(hive.Hook{
		OnStart: func(hive.HookContext) error {
			watchers.PodInit(&wg, clientset, clientset.Slim(), context.Background())
			wg.Done()
			return nil
		},
		OnStop: func(ctx hive.HookContext) error {
			wg.Wait()
			return nil
		},
	})
}
