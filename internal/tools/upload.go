package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ngxuanth/mcp-server/internal/bridge"
	"github.com/ngxuanth/mcp-server/internal/mcp"
)

func uploadFile(hub *bridge.Hub) mcp.Tool {
	return mcp.Tool{
		Name:        "browser_upload_file",
		Description: "Attach a local file to an input[type=file]. Only files inside the directory set by BMCP_UPLOAD_DIR can be attached.",
		InputSchema: objSchema(map[string]any{
			"selector": strProp("CSS selector of the file input, e.g. input[type=file]"),
			"filePath": strProp("Path of the file to attach, relative to BMCP_UPLOAD_DIR"),
		}, "selector", "filePath"),
		Handle: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			var a struct {
				Selector string `json:"selector"`
				FilePath string `json:"filePath"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return mcp.Result{}, err
			}
			absPath, err := resolveUploadPath(a.FilePath)
			if err != nil {
				return mcp.Result{}, err
			}
			if _, err := hub.Send(ctx, "browser_upload_file", map[string]any{
				"selector": a.Selector,
				"filePath": absPath,
			}); err != nil {
				return mcp.Result{}, err
			}
			return mcp.Text("Attached " + quote(a.FilePath) + " to " + quote(a.Selector)), nil
		},
	}
}

// resolveUploadPath resolves filePath against BMCP_UPLOAD_DIR and rejects
// anything that escapes it (via .., absolute paths or symlinks), hidden files,
// and non-regular files. It returns the real absolute path for the browser.
func resolveUploadPath(filePath string) (string, error) {
	uploadDir := os.Getenv("BMCP_UPLOAD_DIR")
	if uploadDir == "" {
		return "", errors.New("File upload is disabled. Set BMCP_UPLOAD_DIR to the directory whose files may be uploaded.")
	}

	realDir, err := filepath.EvalSymlinks(uploadDir)
	if err != nil {
		return "", errors.New("BMCP_UPLOAD_DIR does not exist: " + uploadDir)
	}
	if info, err := os.Stat(realDir); err != nil || !info.IsDir() {
		return "", errors.New("BMCP_UPLOAD_DIR is not a directory: " + uploadDir)
	}

	realFile, err := filepath.EvalSymlinks(filepath.Join(realDir, filePath))
	if err != nil {
		return "", errors.New("File not found in upload directory: " + filePath)
	}

	rel, err := filepath.Rel(realDir, realFile)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("File is outside the upload directory: " + filePath)
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			return "", errors.New("Hidden files cannot be uploaded: " + filePath)
		}
	}
	if info, err := os.Stat(realFile); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Not a regular file: " + filePath)
	}

	return realFile, nil
}
