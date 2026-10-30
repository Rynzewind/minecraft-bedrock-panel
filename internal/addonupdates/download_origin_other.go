//go:build !windows

package addonupdates

func DownloadFileID(string) int64 { return 0 }
