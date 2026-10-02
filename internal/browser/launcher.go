// Package browser optionally starts Chrome with the Browser MCP extension
// loaded and connects it to a tab, doing what clicking "Connect" in the
// extension popup would. It is a port of docker/launcher from the Node project,
// turned into an opt-in feature: it only runs when --launch=chrome is given.
package browser

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/ngxuanth/mcp-server/internal/config"
)

// Launch starts Chrome, waits for the extension to load, connects it to a tab at
// StartURL, and blocks until ctx is cancelled or Chrome exits. It only supports
// Chrome; the extension directory (cfg.ExtensionDir) is required.
func Launch(ctx context.Context, cfg config.Config) error {
	if cfg.ExtensionDir == "" {
		return errors.New("launching chrome requires an unpacked extension: set BMCP_EXTENSION_DIR or --extension-dir")
	}

	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(1280, 800),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("load-extension", cfg.ExtensionDir),
		chromedp.Flag("disable-extensions-except", cfg.ExtensionDir),
		// Recent Chrome builds ignore --load-extension unless this is disabled.
		chromedp.Flag("disable-features", "DisableLoadExtensionCommandLineSwitch"),
	}
	if cfg.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ChromePath))
	}
	if cfg.ProfileDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.ProfileDir))
	}
	// Headless is off by default so the user can watch the automation; set
	// BMCP_HEADLESS=new to run without a window.
	if cfg.Headless != "" {
		opts = append(opts, chromedp.Flag("headless", cfg.Headless))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelBrowser()

	if err := chromedp.Run(browserCtx); err != nil {
		return fmt.Errorf("start chrome: %w", err)
	}

	extID, err := waitForExtension(browserCtx, 30*time.Second)
	if err != nil {
		return fmt.Errorf("find extension: %w", err)
	}
	log.Printf("extension loaded: %s", extID)

	tabID, err := connectTab(browserCtx, extID, cfg.StartURL)
	if err != nil {
		return fmt.Errorf("connect tab: %w", err)
	}
	log.Printf("extension connected to tab %d (%s)", tabID, cfg.StartURL)

	// chromedp cancels browserCtx when Chrome exits.
	<-browserCtx.Done()
	if ctx.Err() == nil {
		return errors.New("chrome exited")
	}
	return nil
}

// waitForExtension returns the ID of the extension whose service worker is
// running, polling until it starts.
func waitForExtension(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		targets, err := chromedp.Targets(ctx)
		if err != nil {
			return "", err
		}
		for _, t := range targets {
			if t.Type == "service_worker" && strings.HasPrefix(t.URL, "chrome-extension://") {
				host := strings.TrimPrefix(t.URL, "chrome-extension://")
				return strings.SplitN(host, "/", 2)[0], nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", errors.New("the extension's service worker did not start; is the extension directory an unpacked extension?")
}

// connectTab opens startURL in a new tab and stores it as the extension's
// selected tab, from an extension page so the chrome.* APIs are available.
func connectTab(ctx context.Context, extID, startURL string) (int64, error) {
	script := fmt.Sprintf(`(async () => {
		const tab = await chrome.tabs.create({ url: %q, active: true });
		await chrome.storage.local.set({ selectedTabId: tab.id });
		return tab.id;
	})()`, startURL)

	var tabID int64
	err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("chrome-extension://%s/popup.html", extID)),
		chromedp.Evaluate(script, &tabID, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	return tabID, err
}
