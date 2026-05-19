// Package clipboard is the platform clipboard adapter: pbcopy on macOS,
// wl-copy (preferred) or xclip on Linux, and clip on Windows. The payload
// is always passed via stdin so finding text is never shell-expanded.
package clipboard
