package healthcheck

import (
	"context"
	"net/url"
)

type Report struct {
	AccessURL AccessURLReport
	Websocket WebsocketReport
	DERP      DERPReport
}

func Run(ctx context.Context, deploymentURL *url.URL) {}
