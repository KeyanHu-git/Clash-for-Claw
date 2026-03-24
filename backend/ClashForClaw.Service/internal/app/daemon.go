package app

import (
	"context"
	"time"

	"clash-for-claw-service/internal/api"
	"clash-for-claw-service/internal/runtime"
)

type Daemon struct {
	server *api.Server
}

func NewDaemon(paths runtime.Paths, allowSelfShutdown bool) (*Daemon, error) {
	server, err := api.NewServer(paths, allowSelfShutdown)
	if err != nil {
		return nil, err
	}
	return &Daemon{server: server}, nil
}

func (d *Daemon) Start() error {
	return d.server.Start()
}

func (d *Daemon) Wait() error {
	return d.server.Wait()
}

func (d *Daemon) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return d.server.Shutdown(ctx)
}
