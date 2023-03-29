package healthcheck_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/coder/coder/coderd/healthcheck"
)

func TestDERP(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = context.Background()
			report = healthcheck.DERPReport{}
			// derpURL, _ = url.Parse("https://derp9d.tailscale.com")
			// derpURL, _ = url.Parse("https://fccab93dfdd255937ad753332a7c13cf.pit-1.try.coder.app")
			derpURL, _ = url.Parse("https://dev.coder.com")
		)
		report.Run(ctx, derpURL)
		spew.Dump(report)
	})
}
