package healthcheck

import (
	"context"
	"fmt"
	"net/url"

	"golang.org/x/xerrors"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"
)

type DERPReport struct {
	Logs []string
	Errs []error
}

func (r *DERPReport) Run(ctx context.Context, accessURL *url.URL) {
	derpURL, err := accessURL.Parse("/derp")
	if err != nil {
		r.Errs = append(r.Errs, xerrors.Errorf("parse derp endpoint: %w", err))
		return
	}

	client, err := derphttp.NewClient(key.NewNode(), derpURL.String(), func(format string, args ...any) {
		r.Logs = append(r.Logs, fmt.Sprintf(format, args...))
	})
	if err != nil {
		r.Errs = append(r.Errs, xerrors.Errorf("create derp client: %w", err))
		return
	}
	defer client.Close()

	for i := 0; i < 5; i++ {
		err := client.Connect(ctx)
		if err != nil {
			r.Errs = append(r.Errs, xerrors.Errorf("connect to derp: %w", err))
			continue
		}
		break
	}
}
