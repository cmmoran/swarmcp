package content

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/shell"
)

const (
	FileScheme   = "file://"
	Base64Scheme = "b64:"
	EnvScheme    = "env:"
)

// FileOrContent holds a file path or content.
type FileOrContent string

// String returns the FileOrContent in string format.
func (f FileOrContent) String() string {
	return string(f)
}

// IsPath returns true if the FileOrContent is a file path, otherwise returns false.
func (f FileOrContent) IsPath(fsys ...fs.FS) bool {
	_, isPath, _ := f.isPath(fsys...)
	return isPath
}

func (f FileOrContent) isPath(fsys ...fs.FS) (string, bool, fs.FS) {
	if len(fsys) == 0 {
		fsys = []fs.FS{
			os.DirFS("/"),
		}
	}
	fstr := f.String()
	if !withinPathLenBounds(fstr) {
		return "", false, nil
	}
	fstr = strings.TrimPrefix(fstr, FileScheme)
	fstr = path.Clean(fstr)
	if strings.HasPrefix(fstr, "/") {
		fstr = fstr[1:]
	}
	var err error
	for _, cfsys := range fsys {
		if _, cerr := fs.Stat(cfsys, fstr); cerr != nil {
			err = errors.Join(err, cerr)
		} else {
			return fstr, true, cfsys
		}
	}

	return "", false, nil
}

func (f FileOrContent) IsEnv() bool {
	return strings.HasPrefix(f.String(), EnvScheme)
}

func (f FileOrContent) IsBase64() bool {
	return strings.HasPrefix(f.String(), Base64Scheme)
}

func (f FileOrContent) MustRead(fsys ...fs.FS) []byte {
	content, _ := f.Read(fsys...)
	return content
}

func (f FileOrContent) MustReadString(fsys ...fs.FS) string {
	content, _ := f.Read(fsys...)
	return string(content)
}

// Read returns the content after reading the FileOrContent variable.
func (f FileOrContent) Read(fsys ...fs.FS) ([]byte, error) {
	var content []byte
	if p, isPath, cfsys := f.isPath(fsys...); isPath {
		var err error
		if content, err = fs.ReadFile(cfsys, p); err == nil {
			return content, nil
		}
		return []byte(f), err
	} else if f.IsEnv() {
		fstr := f.String()
		fstr = strings.TrimPrefix(fstr, EnvScheme)
		if envVal, err := recursiveExpand(fstr); err != nil {
			return []byte(f), err
		} else {
			content = []byte(envVal)
		}
	} else {
		fstr := f.String()
		if f.IsBase64() {
			fstr = fstr[4:]
			if decodedContent, err := base64.StdEncoding.DecodeString(fstr); err != nil {
				return []byte(f), err
			} else {
				content = decodedContent
			}
		} else {
			content = []byte(fstr)
		}
	}
	return content, nil
}

// withinPathLenBounds returns true if the given path string is within
// a safe limit for the current OS and filesystem conventions.
func withinPathLenBounds(fstr string) bool {
	switch strings.ToLower(runtime.GOOS) {

	case "windows":
		// Windows has two modes:
		//   - Normal paths (MAX_PATH = 260)
		//   - Extended paths (\\?\ prefix, up to 32,767 chars)
		// See: https://learn.microsoft.com/en-us/windows/win32/fileio/maximum-file-path-limitation
		if strings.HasPrefix(fstr, `\\?\`) {
			// Extended-length path — 32,767 chars max
			return len(fstr) <= 32767
		}
		// Normal Win32 path — 260 chars max (including drive + NUL terminator)
		return len(fstr) <= 260

	case "darwin":
		// macOS (APFS/HFS+): PATH_MAX ~1024 bytes
		return len(fstr) <= 1024

	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		// Linux / Unix: POSIX PATH_MAX typically 4096 bytes
		return len(fstr) <= 4096

	default:
		// Conservative fallback for unknown OS
		return len(fstr) <= 255
	}
}

func recursiveExpand(v string) (expanded string, err error) {
	expanded, err = shell.Expand(v, nil)
	if err != nil {
		return
	}
	_tmp := expanded
	expanded, err = shell.Expand(_tmp, nil)
	if err != nil {
		return
	}
	if expanded != _tmp {
		return recursiveExpand(expanded)
	}

	return
}
