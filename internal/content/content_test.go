package content

import (
	"embed"
	"os"
	"reflect"
	"testing"
)

//go:embed testdata/filecontent.txt
var testFS embed.FS

func TestFileOrContent_String(t *testing.T) {
	_ = os.Setenv("TEST_ENV_VAR", "test")
	tests := []struct {
		name string
		f    FileOrContent
		want string
	}{
		{
			name: "it should return a string if the content is a string",
			f:    FileOrContent("test"),
			want: "test",
		},
		{
			name: "it should return a string representation even if the content is a filename",
			f:    FileOrContent("/testdata/filecontent.txt"),
			want: "/testdata/filecontent.txt",
		},
		{
			name: "it should return a string representation even if the content is a filename with the file:// prefix",
			f:    FileOrContent("file:///testdata/filecontent.txt"),
			want: "file:///testdata/filecontent.txt",
		},
		{
			name: "it should return the string representation of the environment variable if the content starts with 'env:' and is a valid environment variable",
			f:    FileOrContent("env:TEST_ENV_VAR"),
			want: "env:TEST_ENV_VAR",
		},
		{
			name: "it should return the content string if the content starts with 'env:' but is not a valid environment variable",
			f:    FileOrContent("env:NON_EXISTENT_ENV_VAR"),
			want: "env:NON_EXISTENT_ENV_VAR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileOrContent_IsPath(t *testing.T) {
	exceptionallyLongPath := "a" + string(make([]byte, 4096)) + "b"
	tests := []struct {
		name string
		f    FileOrContent
		want bool
	}{
		{
			name: "it should return false if the content is a non-path string",
			f:    FileOrContent("test"),
			want: false,
		},
		{
			name: "it should return false if the content is longer than the max path length",
			f:    FileOrContent(exceptionallyLongPath),
			want: false,
		},
		{
			name: "it should return true if the content is a path string",
			f:    FileOrContent("testdata/filecontent.txt"),
			want: true,
		},
		{
			name: "it should return true if the content is a path string with leading slash",
			f:    FileOrContent("/testdata/filecontent.txt"),
			want: true,
		},
		{
			name: "it should return true if the content is a path string with the file:// prefix",
			f:    FileOrContent("file://testdata/filecontent.txt"),
			want: true,
		},
		{
			name: "it should return false if the content starts with 'env:' and is a valid environment variable",
			f:    FileOrContent("env:TEST_ENV_VAR"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.IsPath(testFS); got != tt.want {
				t.Errorf("IsPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileOrContent_Read(t *testing.T) {
	t.Setenv("TEST_ENV_VAR", "test")
	tests := []struct {
		name    string
		f       FileOrContent
		want    []byte
		wantErr bool
	}{
		{
			name: "it should return the content if the content is a string",
			f:    FileOrContent("test"),
			want: []byte("test"),
		},
		{
			name: "it should return the content if the content is a filename",
			f:    FileOrContent("testdata/filecontent.txt"),
			want: []byte("test file content\n"),
		},
		{
			name: "it should return the content if the content is a filename with relative path",
			f:    FileOrContent("./testdata/filecontent.txt"),
			want: []byte("test file content\n"),
		},
		{
			name: "it should return the content if the content is a filename but file does not exist",
			f:    FileOrContent("testdata/otherfilecontent.txt"),
			want: []byte("testdata/otherfilecontent.txt"),
		},
		{
			name: "it should return the content if the content is a filename but file does not exist with the file:// prefix",
			f:    FileOrContent("file://testdata/otherfilecontent.txt"),
			want: []byte("file://testdata/otherfilecontent.txt"),
		},
		{
			name: "it should return the content if the content is a filename with the file:// prefix",
			f:    FileOrContent("file://testdata/filecontent.txt"),
			want: []byte("test file content\n"),
		},
		{
			name: "it should return the content if the content is a filename with leading slash",
			f:    FileOrContent("/testdata/filecontent.txt"),
			want: []byte("test file content\n"),
		},
		{
			name: "it should return the content if the content is a filename with the file:// prefix and leading slash",
			f:    FileOrContent("file:///testdata/filecontent.txt"),
			want: []byte("test file content\n"),
		},
		{
			name: "it should return the decoded base64 content if the content starts with 'b64:' and is valid base64 content",
			f:    FileOrContent("b64:dGVzdCBmaWxlIGNvbnRlbnQK"),
			want: []byte("test file content\n"),
		},
		{
			name:    "it should return the content string if the content starts with 'b64:' but is not valid base64 content",
			f:       FileOrContent("b64:abcd===="),
			want:    []byte("b64:abcd===="),
			wantErr: true,
		},
		{
			name: "it should return the environment variable content if the content starts with 'env:' and is a valid environment variable",
			f:    FileOrContent("env:$TEST_ENV_VAR"),
			want: []byte("test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.f.Read(testFS)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Read() got = %v, want %v", got, tt.want)
			}
		})
	}
}
