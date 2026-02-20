package config

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	gitcfg "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	sshconfig "github.com/kevinburke/ssh_config"
	sshagent "github.com/xanzy/ssh-agent"
	"golang.org/x/crypto/ssh"
)

const gitSourcePrefix = "git:"

var errExactSHA1NotSupported = errors.New("git server does not allow exact object fetch")
var resolvedRefCache sync.Map

// SourceMetadata records the git subtree fingerprint for a path.
type SourceMetadata struct {
	URL       string `json:"url"`
	Ref       string `json:"ref"`
	Commit    string `json:"commit"`
	Path      string `json:"path"`
	Subtree   string `json:"subtree"`
	FetchedAt string `json:"fetched_at"`
}

type gitSource struct {
	URL  string
	Ref  string
	Path string
}

// GitSource is a parsed git source reference.
type GitSource struct {
	URL  string
	Ref  string
	Path string
}

type gitPathResult struct {
	CommitHash  plumbing.Hash
	SubtreeHash plumbing.Hash
	Files       map[string][]byte
	IsTree      bool
}

type sshCommandOptions struct {
	User           string
	IdentityFiles  []string
	IdentitiesOnly *bool
	ConfigFile     string
}

type sshHostConfig struct {
	User           string
	IdentityFiles  []string
	IdentitiesOnly bool
	HostName       string
	Port           string
	Source         string
}

type gitConfigSource struct {
	Precedence int
	Path       string
}

type gitURLRewriteRule struct {
	Base       string
	InsteadOf  string
	Precedence int
	Order      int
}

func FetchRepoRoot(url string, ref string, opts LoadOptions) (string, error) {
	if url == "" {
		return "", fmt.Errorf("source url is required")
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		return "", fmt.Errorf("cache dir is required for repo fetch")
	}
	repoDir := filepath.Join(cacheDir, "repos", hashKey(url))
	if _, err := os.Stat(repoDir); err != nil {
		if opts.Offline {
			return "", fmt.Errorf("repo %q not cached and offline is enabled", url)
		}
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return "", err
		}
		if _, err := git.PlainInit(repoDir, true); err != nil {
			return "", err
		}
	}
	if _, err := git.PlainOpen(repoDir); err != nil {
		return "", err
	}
	debugf(opts, "git: repo cache ready url=%s", url)
	return encodeGitSource(url, ref, ""), nil
}

func ReadSourceFile(path string, baseDir string, opts LoadOptions) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("source path is required")
	}
	resolved := path
	if baseDir != "" && !IsGitSource(resolved) && !filepath.IsAbs(resolved) {
		if IsGitSource(baseDir) {
			var err error
			resolved, err = resolvePathWithin(baseDir, resolved, opts)
			if err != nil {
				return nil, err
			}
		} else {
			resolved = filepath.Join(baseDir, resolved)
		}
	}
	if IsGitSource(resolved) {
		parsed, ok, err := parseGitSource(resolved)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("invalid git source %q", resolved)
		}
		debugf(opts, "git: read path=%s ref=%s", parsed.Path, parsed.Ref)
		result, err := readGitPath(context.Background(), parsed.URL, parsed.Ref, parsed.Path, opts)
		if err != nil {
			return nil, err
		}
		content, ok := result.Files[parsed.Path]
		if !ok {
			return nil, fmt.Errorf("git source %q not found", parsed.Path)
		}
		return content, nil
	}
	return os.ReadFile(resolved)
}

// ReadGitSourceFiles loads file contents for a git source at the given ref.
func ReadGitSourceFiles(url, ref, pathValue string, opts LoadOptions) (map[string][]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("source url is required")
	}
	cleanPath, err := cleanGitPath(pathValue)
	if err != nil {
		return nil, err
	}
	result, err := readGitPath(context.Background(), url, ref, cleanPath, opts)
	if err != nil {
		return nil, err
	}
	return result.Files, nil
}

func IsGitSource(value string) bool {
	return strings.HasPrefix(value, gitSourcePrefix)
}

func resolveGitPathWithin(root string, rel string, opts LoadOptions) (string, error) {
	parsed, ok, err := parseGitSource(root)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("invalid git root %q", root)
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("source path %q escapes git root", rel)
	}
	joined, err := joinGitPath(parsed.Path, rel)
	if err != nil {
		return "", err
	}
	if _, err := readGitPath(context.Background(), parsed.URL, parsed.Ref, joined, opts); err != nil {
		return "", err
	}
	return encodeGitSource(parsed.URL, parsed.Ref, joined), nil
}

func encodeGitSource(rawURL string, ref string, p string) string {
	return gitSourcePrefix + url.QueryEscape(rawURL) + "|" + url.QueryEscape(ref) + "|" + url.QueryEscape(p)
}

// ParseGitSource parses an encoded git source reference.
func ParseGitSource(value string) (GitSource, bool, error) {
	parsed, ok, err := parseGitSource(value)
	if err != nil || !ok {
		return GitSource{}, ok, err
	}
	return GitSource{URL: parsed.URL, Ref: parsed.Ref, Path: parsed.Path}, true, nil
}

func parseGitSource(value string) (gitSource, bool, error) {
	if !IsGitSource(value) {
		return gitSource{}, false, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(value, gitSourcePrefix), "|", 3)
	if len(parts) != 3 {
		return gitSource{}, false, fmt.Errorf("invalid git source %q", value)
	}
	rawURL, err := url.QueryUnescape(parts[0])
	if err != nil {
		return gitSource{}, false, err
	}
	ref, err := url.QueryUnescape(parts[1])
	if err != nil {
		return gitSource{}, false, err
	}
	p, err := url.QueryUnescape(parts[2])
	if err != nil {
		return gitSource{}, false, err
	}
	clean, err := cleanGitPath(p)
	if err != nil {
		return gitSource{}, false, err
	}
	return gitSource{URL: rawURL, Ref: ref, Path: clean}, true, nil
}

// FetchGitSource fetches the git source to the local cache and returns metadata about the fetched content.
func FetchGitSource(url, ref, pathValue string, opts LoadOptions) (SourceMetadata, error) {
	if url == "" {
		return SourceMetadata{}, fmt.Errorf("source url is required")
	}
	cleanPath, err := cleanGitPath(pathValue)
	if err != nil {
		return SourceMetadata{}, err
	}
	result, err := readGitPath(context.Background(), url, ref, cleanPath, opts)
	if err != nil {
		return SourceMetadata{}, err
	}
	if result.CommitHash != plumbing.ZeroHash && opts.CacheDir != "" {
		repoDir := filepath.Join(opts.CacheDir, "repos", hashKey(url))
		if repo, err := git.PlainOpen(repoDir); err == nil {
			rememberRef(repo, ref, result.CommitHash, opts)
		}
	}
	meta, _, err := ReadSourceMetadata(url, ref, cleanPath, opts)
	if err != nil {
		return SourceMetadata{}, err
	}
	return meta, nil
}

// ReadSourceMetadata loads cached git source metadata if available.
func ReadSourceMetadata(url, ref, pathValue string, opts LoadOptions) (SourceMetadata, bool, error) {
	if url == "" {
		return SourceMetadata{}, false, fmt.Errorf("source url is required")
	}
	if opts.CacheDir == "" {
		return SourceMetadata{}, false, fmt.Errorf("cache dir is required for metadata read")
	}
	repoDir := filepath.Join(opts.CacheDir, "repos", hashKey(url))
	metaDir := filepath.Join(repoDir, ".swarmcp_sources")
	metaPath := filepath.Join(metaDir, hashKey(url+"|"+ref+"|"+pathValue)+".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SourceMetadata{}, false, nil
		}
		return SourceMetadata{}, false, err
	}
	var meta SourceMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return SourceMetadata{}, false, err
	}
	return meta, true, nil
}

func cleanGitPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	clean := path.Clean(strings.TrimPrefix(p, "/"))
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("git path %q escapes repository", p)
	}
	return clean, nil
}

func joinGitPath(root string, rel string) (string, error) {
	rootClean, err := cleanGitPath(root)
	if err != nil {
		return "", err
	}
	relClean, err := cleanGitPath(rel)
	if err != nil {
		return "", err
	}
	if rootClean == "" {
		return relClean, nil
	}
	if relClean == "" {
		return rootClean, nil
	}
	joined := path.Join(rootClean, relClean)
	if joined == rootClean || strings.HasPrefix(joined, rootClean+"/") {
		return joined, nil
	}
	return "", fmt.Errorf("git path %q escapes root %q", rel, root)
}

func readGitPath(ctx context.Context, rawURL, ref, p string, opts LoadOptions) (gitPathResult, error) {
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		return gitPathResult{}, fmt.Errorf("cache dir is required for repo fetch")
	}
	remoteURL := rewriteGitURL(rawURL)
	if opts.Debug {
		if remoteURL != rawURL {
			debugf(opts, "git: rewrite url=%s -> %s", rawURL, remoteURL)
		} else {
			debugf(opts, "git: rewrite url=%s (no match)", rawURL)
		}
	}
	repoDir := filepath.Join(cacheDir, "repos", hashKey(rawURL))
	if _, err := os.Stat(repoDir); err != nil {
		if opts.Offline {
			return gitPathResult{}, fmt.Errorf("repo %q not cached and offline is enabled", rawURL)
		}
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return gitPathResult{}, err
		}
		if _, err := git.PlainInit(repoDir, true); err != nil {
			return gitPathResult{}, err
		}
	}
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return gitPathResult{}, err
	}
	debugf(opts, "git: resolve ref=%s url=%s", strings.TrimSpace(ref), remoteURL)
	auth, err := authForURL(remoteURL, opts)
	if err != nil {
		return gitPathResult{}, err
	}
	commitHash, err := resolveRefHash(ctx, repo, remoteURL, ref, auth, opts)
	if err != nil {
		return gitPathResult{}, err
	}
	debugf(opts, "git: commit=%s", commitHash.String())
	commit, err := loadCommit(ctx, repo, remoteURL, auth, commitHash, opts)
	if err != nil {
		return gitPathResult{}, err
	}
	rootTree, err := loadTree(ctx, repo, remoteURL, auth, commit.TreeHash, opts)
	if err != nil {
		return gitPathResult{}, err
	}
	cleanPath, err := cleanGitPath(p)
	if err != nil {
		return gitPathResult{}, err
	}
	result := gitPathResult{
		CommitHash: commitHash,
		Files:      make(map[string][]byte),
	}
	if cleanPath == "" {
		result.SubtreeHash = rootTree.Hash
		result.IsTree = true
		debugf(opts, "git: subtree path=/ hash=%s", rootTree.Hash.String())
		if err := readTreeBlobs(ctx, repo, remoteURL, auth, rootTree, "", result.Files, opts); err != nil {
			return gitPathResult{}, err
		}
		if err := writeSourceMetadata(repoDir, rawURL, ref, commitHash.String(), cleanPath, result.SubtreeHash.String()); err != nil {
			return gitPathResult{}, err
		}
		return result, nil
	}

	entry, entryTree, isTree, err := findTreeEntry(ctx, repo, remoteURL, auth, rootTree, cleanPath, opts)
	if err != nil {
		return gitPathResult{}, err
	}
	if isTree {
		result.SubtreeHash = entryTree.Hash
		result.IsTree = true
		debugf(opts, "git: subtree path=%s hash=%s", cleanPath, entryTree.Hash.String())
		if err := readTreeBlobs(ctx, repo, remoteURL, auth, entryTree, cleanPath, result.Files, opts); err != nil {
			return gitPathResult{}, err
		}
	} else {
		blob, err := loadBlob(ctx, repo, remoteURL, auth, entry.Hash, opts)
		if err != nil {
			return gitPathResult{}, err
		}
		content, err := readBlobContent(blob)
		if err != nil {
			return gitPathResult{}, err
		}
		debugf(opts, "git: blob path=%s hash=%s size=%d", cleanPath, entry.Hash.String(), len(content))
		result.Files[cleanPath] = content
		result.SubtreeHash, err = singleEntryTreeHash(entry)
		if err != nil {
			return gitPathResult{}, err
		}
		result.IsTree = false
	}
	if err := writeSourceMetadata(repoDir, rawURL, ref, commitHash.String(), cleanPath, result.SubtreeHash.String()); err != nil {
		return gitPathResult{}, err
	}
	return result, nil
}

func resolveRefHash(ctx context.Context, repo *git.Repository, rawURL, ref string, auth transport.AuthMethod, opts LoadOptions) (plumbing.Hash, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.EqualFold(ref, "HEAD") {
		if hash, ok := resolveLocalHEAD(repo); ok {
			return hash, nil
		}
		if opts.Offline {
			return plumbing.ZeroHash, fmt.Errorf("ref HEAD not found in local repo and offline is enabled")
		}
		adv, err := advertisedRefs(ctx, rawURL, auth, opts)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if adv.Head == nil {
			return plumbing.ZeroHash, fmt.Errorf("ref HEAD not advertised by remote")
		}
		rememberRef(repo, "HEAD", *adv.Head, opts)
		return *adv.Head, nil
	}
	if plumbing.IsHash(ref) {
		return plumbing.NewHash(ref), nil
	}
	cacheKey := rawURL + "|" + ref
	if !opts.Offline {
		if cached, ok := resolvedRefCache.Load(cacheKey); ok {
			if hash, ok := cached.(plumbing.Hash); ok && hash != plumbing.ZeroHash {
				return hash, nil
			}
		}
	}
	localHash, localOK, err := resolveLocalRef(repo, rawURL, ref, auth, opts)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if opts.Offline {
		if localOK {
			return localHash, nil
		}
		return plumbing.ZeroHash, fmt.Errorf("ref %q not found in local repo and offline is enabled", ref)
	}
	adv, err := advertisedRefs(ctx, rawURL, auth, opts)
	if err != nil {
		if localOK {
			return localHash, nil
		}
		return plumbing.ZeroHash, err
	}
	hash, ok := resolveRefFromAdv(adv, ref)
	if ok {
		rememberRef(repo, ref, hash, opts)
		resolvedRefCache.Store(cacheKey, hash)
		return hash, nil
	}
	if localOK {
		resolvedRefCache.Store(cacheKey, localHash)
		return localHash, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("ref %q not found in remote", ref)
}

func resolveLocalHEAD(repo *git.Repository) (plumbing.Hash, bool) {
	head, err := repo.Head()
	if err != nil {
		if hash, ok := resolveLocalSwarmcpRef(repo, "HEAD"); ok {
			return hash, true
		}
		return plumbing.ZeroHash, false
	}
	return head.Hash(), true
}

func resolveLocalRef(repo *git.Repository, rawURL, ref string, auth transport.AuthMethod, opts LoadOptions) (plumbing.Hash, bool, error) {
	candidates := refCandidates(ref)
	for _, name := range candidates {
		resolved, err := repo.Reference(plumbing.ReferenceName(name), true)
		if err != nil {
			continue
		}
		hash := resolved.Hash()
		peeled, err := peelTag(context.Background(), repo, rawURL, auth, hash, opts)
		if err != nil {
			return plumbing.ZeroHash, false, err
		}
		return peeled, true, nil
	}
	return plumbing.ZeroHash, false, nil
}

func peelTag(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, hash plumbing.Hash, opts LoadOptions) (plumbing.Hash, error) {
	tag, err := repo.TagObject(hash)
	if err == nil {
		switch tag.TargetType {
		case plumbing.CommitObject:
			return tag.Target, nil
		case plumbing.TagObject:
			return peelTag(ctx, repo, rawURL, auth, tag.Target, opts)
		default:
			return tag.Target, nil
		}
	}
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		if opts.Offline {
			return hash, nil
		}
		if err := ensureObject(ctx, repo, rawURL, auth, hash, opts); err != nil {
			return plumbing.ZeroHash, err
		}
		// Not a tag object; return the original hash after ensuring it's present.
		return hash, nil
	}
	return hash, nil
}

func resolveRefFromAdv(adv *packp.AdvRefs, ref string) (plumbing.Hash, bool) {
	for _, name := range refCandidates(ref) {
		if hash, ok := adv.References[name]; ok {
			if peeled, ok := adv.Peeled[name]; ok {
				return peeled, true
			}
			return hash, true
		}
	}
	return plumbing.ZeroHash, false
}

const swarmcpRefPrefix = "refs/swarmcp/"

func refCandidates(ref string) []string {
	if strings.HasPrefix(ref, "refs/") {
		return []string{ref}
	}
	return []string{
		swarmcpRefPrefix + ref,
		"refs/heads/" + ref,
		"refs/tags/" + ref,
	}
}

func resolveLocalSwarmcpRef(repo *git.Repository, ref string) (plumbing.Hash, bool) {
	if strings.TrimSpace(ref) == "" {
		return plumbing.ZeroHash, false
	}
	name := plumbing.ReferenceName(swarmcpRefPrefix + ref)
	resolved, err := repo.Reference(name, true)
	if err != nil {
		return plumbing.ZeroHash, false
	}
	return resolved.Hash(), true
}

func rememberRef(repo *git.Repository, ref string, hash plumbing.Hash, opts LoadOptions) {
	if hash == plumbing.ZeroHash {
		return
	}
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.EqualFold(ref, "HEAD") {
		name := plumbing.ReferenceName(swarmcpRefPrefix + "HEAD")
		_ = repo.Storer.SetReference(plumbing.NewHashReference(name, hash))
		return
	}
	if plumbing.IsHash(ref) {
		return
	}
	var name plumbing.ReferenceName
	if strings.HasPrefix(ref, "refs/") {
		name = plumbing.ReferenceName(ref)
	} else {
		name = plumbing.ReferenceName(swarmcpRefPrefix + ref)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(name, hash)); err == nil {
		debugf(opts, "git: remember ref=%s hash=%s", name.String(), hash.String())
	}
}

func loadCommit(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, hash plumbing.Hash, opts LoadOptions) (*object.Commit, error) {
	if err := ensureObject(ctx, repo, rawURL, auth, hash, opts); err != nil {
		return nil, err
	}
	return object.GetCommit(repo.Storer, hash)
}

func loadTree(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, hash plumbing.Hash, opts LoadOptions) (*object.Tree, error) {
	if err := ensureObject(ctx, repo, rawURL, auth, hash, opts); err != nil {
		return nil, err
	}
	return object.GetTree(repo.Storer, hash)
}

func loadBlob(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, hash plumbing.Hash, opts LoadOptions) (*object.Blob, error) {
	if err := ensureObject(ctx, repo, rawURL, auth, hash, opts); err != nil {
		return nil, err
	}
	return object.GetBlob(repo.Storer, hash)
}

func ensureObject(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, hash plumbing.Hash, opts LoadOptions) error {
	if hash == plumbing.ZeroHash {
		return fmt.Errorf("object hash is empty")
	}
	_, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
	if err == nil {
		return nil
	}
	if !errors.Is(err, plumbing.ErrObjectNotFound) {
		return err
	}
	if opts.Offline {
		return fmt.Errorf("object %s not found in cache and offline is enabled", hash.String())
	}
	return fetchObjects(ctx, repo, rawURL, auth, []plumbing.Hash{hash}, opts)
}

func findTreeEntry(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, tree *object.Tree, p string, opts LoadOptions) (object.TreeEntry, *object.Tree, bool, error) {
	parts := strings.Split(p, "/")
	current := tree
	for i, part := range parts {
		entry, err := findEntry(current, part)
		if err != nil {
			return object.TreeEntry{}, nil, false, fmt.Errorf("path %q not found", p)
		}
		if i == len(parts)-1 {
			if entry.Mode == filemode.Dir {
				subtree, err := loadTree(ctx, repo, rawURL, auth, entry.Hash, opts)
				if err != nil {
					return object.TreeEntry{}, nil, false, err
				}
				return *entry, subtree, true, nil
			}
			return *entry, nil, false, nil
		}
		if entry.Mode != filemode.Dir {
			return object.TreeEntry{}, nil, false, fmt.Errorf("path %q is not a directory", path.Join(parts[:i+1]...))
		}
		nextTree, err := loadTree(ctx, repo, rawURL, auth, entry.Hash, opts)
		if err != nil {
			return object.TreeEntry{}, nil, false, err
		}
		current = nextTree
	}
	return object.TreeEntry{}, nil, false, fmt.Errorf("path %q not found", p)
}

func findEntry(tree *object.Tree, name string) (*object.TreeEntry, error) {
	for i := range tree.Entries {
		if tree.Entries[i].Name == name {
			return &tree.Entries[i], nil
		}
	}
	return nil, object.ErrEntryNotFound
}

func readTreeBlobs(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, tree *object.Tree, prefix string, out map[string][]byte, opts LoadOptions) error {
	for _, entry := range tree.Entries {
		entryPath := entry.Name
		if prefix != "" {
			entryPath = path.Join(prefix, entry.Name)
		}
		switch entry.Mode {
		case filemode.Dir:
			subtree, err := loadTree(ctx, repo, rawURL, auth, entry.Hash, opts)
			if err != nil {
				return err
			}
			if err := readTreeBlobs(ctx, repo, rawURL, auth, subtree, entryPath, out, opts); err != nil {
				return err
			}
		case filemode.Regular, filemode.Executable, filemode.Deprecated, filemode.Symlink:
			blob, err := loadBlob(ctx, repo, rawURL, auth, entry.Hash, opts)
			if err != nil {
				return err
			}
			content, err := readBlobContent(blob)
			if err != nil {
				return err
			}
			out[entryPath] = content
		case filemode.Submodule:
			return fmt.Errorf("submodule %q is not supported", entryPath)
		default:
			return fmt.Errorf("unsupported git entry %q mode %v", entryPath, entry.Mode)
		}
	}
	return nil
}

func readBlobContent(blob *object.Blob) ([]byte, error) {
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func singleEntryTreeHash(entry object.TreeEntry) (plumbing.Hash, error) {
	tree := object.Tree{Entries: []object.TreeEntry{entry}}
	obj := &plumbing.MemoryObject{}
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return obj.Hash(), nil
}

func advertisedRefs(ctx context.Context, rawURL string, auth transport.AuthMethod, opts LoadOptions) (*packp.AdvRefs, error) {
	c, ep, err := newClient(rawURL, opts)
	if err != nil {
		return nil, err
	}
	session, err := c.NewUploadPackSession(ep, auth)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.AdvertisedReferencesContext(ctx)
}

func fetchObjects(ctx context.Context, repo *git.Repository, rawURL string, auth transport.AuthMethod, wants []plumbing.Hash, opts LoadOptions) error {
	if len(wants) == 0 {
		return nil
	}
	c, ep, err := newClient(rawURL, opts)
	if err != nil {
		return err
	}
	session, err := c.NewUploadPackSession(ep, auth)
	if err != nil {
		return err
	}
	defer session.Close()
	adv, err := session.AdvertisedReferencesContext(ctx)
	if err != nil {
		return err
	}
	if !adv.Capabilities.Supports(capability.AllowReachableSHA1InWant) && !adv.Capabilities.Supports(capability.AllowTipSHA1InWant) {
		return errExactSHA1NotSupported
	}
	debugf(opts, "git: fetch objects wants=%d", len(wants))
	req := packp.NewUploadPackRequestFromCapabilities(adv.Capabilities)
	req.Wants = wants
	reader, err := session.UploadPack(ctx, req)
	if err != nil {
		if errors.Is(err, transport.ErrEmptyUploadPackRequest) {
			return nil
		}
		return err
	}
	defer reader.Close()
	return packfile.UpdateObjectStorage(repo.Storer, buildSidebandIfSupported(req.Capabilities, reader))
}

func buildSidebandIfSupported(l *capability.List, reader io.Reader) io.Reader {
	var t sideband.Type
	if l.Supports(capability.Sideband) {
		t = sideband.Sideband
	} else if l.Supports(capability.Sideband64k) {
		t = sideband.Sideband64k
	} else {
		return reader
	}
	return sideband.NewDemuxer(t, reader)
}

func newClient(rawURL string, opts LoadOptions) (transport.Transport, *transport.Endpoint, error) {
	ep, err := transport.NewEndpoint(rawURL)
	if err != nil {
		return nil, nil, err
	}
	ep, err = resolveSSHAliasEndpoint(ep, parseGitSSHCommand().ConfigFile, opts)
	if err != nil {
		return nil, nil, err
	}
	c, err := client.NewClient(ep)
	if err != nil {
		return nil, nil, err
	}
	return c, ep, nil
}

func authForURL(rawURL string, opts LoadOptions) (transport.AuthMethod, error) {
	ep, err := transport.NewEndpoint(rawURL)
	if err != nil {
		return nil, err
	}
	switch ep.Protocol {
	case "ssh", "git+ssh":
		auth, err := sshAuth(ep, opts)
		if err != nil {
			return nil, err
		}
		return auth, nil
	case "http", "https":
		if auth, ok := netrcAuth(ep); ok {
			debugf(opts, "git: auth=http netrc")
			return auth, nil
		}
		if auth, ok, err := credentialHelperAuth(rawURL, ep); err != nil {
			return nil, err
		} else if ok {
			debugf(opts, "git: auth=http credential-helper")
			return auth, nil
		}
		debugf(opts, "git: auth=http none")
		return nil, nil
	default:
		return nil, nil
	}
}

type netrcEntry struct {
	Login    string
	Password string
}

func netrcAuth(ep *transport.Endpoint) (transport.AuthMethod, bool) {
	entries := readNetrc()
	host := strings.Trim(ep.Host, "[]")
	entry, ok := entries[host]
	if !ok {
		entry, ok = entries["default"]
		if !ok {
			return nil, false
		}
	}
	if entry.Login == "" {
		return nil, false
	}
	return &githttp.BasicAuth{Username: entry.Login, Password: entry.Password}, true
}

func readNetrc() map[string]netrcEntry {
	path := os.Getenv("NETRC")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		path = filepath.Join(home, ".netrc")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	entries := make(map[string]netrcEntry)
	for i := 0; i < len(fields); {
		switch fields[i] {
		case "machine":
			if i+1 >= len(fields) {
				return entries
			}
			host := fields[i+1]
			i += 2
			var entry netrcEntry
			for i < len(fields) {
				switch fields[i] {
				case "login":
					if i+1 < len(fields) {
						entry.Login = fields[i+1]
						i += 2
						continue
					}
				case "password":
					if i+1 < len(fields) {
						entry.Password = fields[i+1]
						i += 2
						continue
					}
				case "machine", "default":
					entries[host] = entry
					goto next
				}
				i++
			}
			entries[host] = entry
		case "default":
			i++
			var entry netrcEntry
			for i < len(fields) {
				switch fields[i] {
				case "login":
					if i+1 < len(fields) {
						entry.Login = fields[i+1]
						i += 2
						continue
					}
				case "password":
					if i+1 < len(fields) {
						entry.Password = fields[i+1]
						i += 2
						continue
					}
				case "machine", "default":
					entries["default"] = entry
					goto next
				}
				i++
			}
			entries["default"] = entry
		default:
			i++
		}
	next:
	}
	return entries
}

func credentialHelperAuth(rawURL string, ep *transport.Endpoint) (transport.AuthMethod, bool, error) {
	helpers := credentialHelpers(rawURL)
	for _, helper := range helpers {
		user, pass, ok, err := runCredentialHelper(helper, ep)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return &githttp.BasicAuth{Username: user, Password: pass}, true, nil
		}
	}
	return nil, false, nil
}

func credentialHelpers(rawURL string) []string {
	cfgs := loadGitConfigs()
	var helpers []string
	for _, cfg := range cfgs {
		if cfg == nil {
			continue
		}
		section := cfg.Section("credential")
		if section != nil {
			helpers = append(helpers, section.OptionAll("helper")...)
			for _, subsection := range section.Subsections {
				if strings.HasPrefix(rawURL, subsection.Name) {
					helpers = append(helpers, subsection.Options.GetAll("helper")...)
				}
			}
		}
	}
	return helpers
}

func rewriteGitURL(rawURL string) string {
	rules := gitURLRewriteRules()
	if len(rules) == 0 {
		return rawURL
	}
	bestIdx := -1
	bestPrefixLen := -1
	bestPrecedence := -1
	bestOrder := -1
	for i, rule := range rules {
		if rule.InsteadOf == "" || rule.Base == "" {
			continue
		}
		if !strings.HasPrefix(rawURL, rule.InsteadOf) {
			continue
		}
		pLen := len(rule.InsteadOf)
		if pLen > bestPrefixLen ||
			(pLen == bestPrefixLen && rule.Precedence > bestPrecedence) ||
			(pLen == bestPrefixLen && rule.Precedence == bestPrecedence && rule.Order > bestOrder) {
			bestIdx = i
			bestPrefixLen = pLen
			bestPrecedence = rule.Precedence
			bestOrder = rule.Order
		}
	}
	if bestIdx < 0 {
		return rawURL
	}
	rule := rules[bestIdx]
	return rule.Base + rawURL[len(rule.InsteadOf):]
}

func gitURLRewriteRules() []gitURLRewriteRule {
	sources := gitConfigSources()
	rules := make([]gitURLRewriteRule, 0)
	order := 0
	for _, src := range sources {
		if src.Path == "" {
			continue
		}
		f, err := os.Open(src.Path)
		if err != nil {
			continue
		}
		cfg := gitcfg.New()
		decodeErr := gitcfg.NewDecoder(f).Decode(cfg)
		_ = f.Close()
		if decodeErr != nil {
			continue
		}
		section := cfg.Section("url")
		if section == nil {
			continue
		}
		for _, subsection := range section.Subsections {
			base := strings.TrimSpace(subsection.Name)
			if base == "" {
				continue
			}
			for _, instead := range subsection.Options.GetAll("insteadOf") {
				instead = strings.TrimSpace(instead)
				if instead == "" {
					continue
				}
				rules = append(rules, gitURLRewriteRule{
					Base:       base,
					InsteadOf:  instead,
					Precedence: src.Precedence,
					Order:      order,
				})
				order++
			}
		}
	}
	return rules
}

func gitConfigSources() []gitConfigSource {
	sources := make([]gitConfigSource, 0, 8)
	seen := map[string]struct{}{}
	add := func(precedence int, p string) {
		p = strings.TrimSpace(p)
		if p == "" || p == os.DevNull {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		sources = append(sources, gitConfigSource{Precedence: precedence, Path: p})
	}

	// System (lowest precedence).
	if system := strings.TrimSpace(os.Getenv("GIT_CONFIG_SYSTEM")); system != "" {
		add(1, system)
	} else {
		add(1, "/etc/gitconfig")
	}

	// Global.
	if global := strings.TrimSpace(os.Getenv("GIT_CONFIG_GLOBAL")); global != "" {
		add(2, global)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			add(2, filepath.Join(home, ".gitconfig"))
			xdg := os.Getenv("XDG_CONFIG_HOME")
			if xdg == "" {
				xdg = filepath.Join(home, ".config")
			}
			add(2, filepath.Join(xdg, "git", "config"))
		}
	}

	// Local (highest precedence).
	if local := strings.TrimSpace(os.Getenv("GIT_CONFIG_LOCAL")); local != "" {
		add(3, local)
	} else {
		if gitDir := strings.TrimSpace(os.Getenv("GIT_DIR")); gitDir != "" {
			add(3, filepath.Join(gitDir, "config"))
		}
		if wd, err := os.Getwd(); err == nil {
			if local, ok := findLocalGitConfig(wd); ok {
				add(3, local)
			}
		}
	}
	return sources
}

func findLocalGitConfig(start string) (string, bool) {
	dir := start
	for {
		if dir == "" {
			return "", false
		}
		candidate := filepath.Join(dir, ".git", "config")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func loadGitConfigs() []*gitcfg.Config {
	paths := gitConfigPaths()
	configs := make([]*gitcfg.Config, 0, len(paths))
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		cfg := gitcfg.New()
		if err := gitcfg.NewDecoder(f).Decode(cfg); err != nil {
			_ = f.Close()
			continue
		}
		_ = f.Close()
		configs = append(configs, cfg)
	}
	return configs
}

func gitConfigPaths() []string {
	var paths []string
	if path := os.Getenv("GIT_CONFIG_GLOBAL"); path != "" {
		paths = append(paths, path)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths, filepath.Join(home, ".gitconfig"))
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		paths = append(paths, filepath.Join(xdg, "git", "config"))
	}
	return paths
}

func runCredentialHelper(helper string, ep *transport.Endpoint) (string, string, bool, error) {
	helper = strings.TrimSpace(helper)
	if helper == "" {
		return "", "", false, nil
	}
	cmd, args := helperCommand(helper)
	if cmd == "" {
		return "", "", false, nil
	}
	request := fmt.Sprintf("protocol=%s\nhost=%s\npath=%s\n\n", ep.Protocol, strings.Trim(ep.Host, "[]"), strings.TrimPrefix(ep.Path, "/"))
	c := exec.Command(cmd, args...)
	c.Stdin = strings.NewReader(request)
	out, err := c.Output()
	if err != nil {
		return "", "", false, nil
	}
	user, pass := parseCredentialOutput(out)
	if user == "" && pass == "" {
		return "", "", false, nil
	}
	return user, pass, true, nil
}

func helperCommand(helper string) (string, []string) {
	if strings.HasPrefix(helper, "!") {
		return "sh", []string{"-c", strings.TrimPrefix(helper, "!") + " get"}
	}
	if strings.Contains(helper, "/") || strings.Contains(helper, "\\") {
		return helper, []string{"get"}
	}
	return "git-credential-" + helper, []string{"get"}
}

func parseCredentialOutput(data []byte) (string, string) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var user, pass string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "username=") {
			user = strings.TrimPrefix(line, "username=")
		}
		if strings.HasPrefix(line, "password=") {
			pass = strings.TrimPrefix(line, "password=")
		}
	}
	return user, pass
}

func debugf(opts LoadOptions, format string, args ...any) {
	if !opts.Debug {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}

func sshAuth(ep *transport.Endpoint, opts LoadOptions) (transport.AuthMethod, error) {
	user := ep.User
	host := strings.Trim(ep.Host, "[]")
	sshCmd := parseGitSSHCommand()
	if opts.Debug && sshCmd.ConfigFile != "" {
		debugf(opts, "git: ssh command config=%s", sshCmd.ConfigFile)
	}
	cfg := sshConfig(host, sshCmd.ConfigFile)
	if opts.Debug {
		sourceLabel := cfg.Source
		if sourceLabel == "" {
			sourceLabel = "default"
		}
		debugf(opts, "git: ssh config source=%s host=%s user=%s identities_only=%t identity_files=%v", sourceLabel, host, cfg.User, cfg.IdentitiesOnly, cfg.IdentityFiles)
	}
	if user == "" {
		user = cfg.User
	}
	if sshCmd.User != "" {
		user = sshCmd.User
	}
	if user == "" {
		user = "git"
	}

	identityFiles := cfg.IdentityFiles
	if len(sshCmd.IdentityFiles) > 0 {
		identityFiles = append(sshCmd.IdentityFiles, identityFiles...)
	}
	identitiesOnly := cfg.IdentitiesOnly
	if sshCmd.IdentitiesOnly != nil {
		identitiesOnly = *sshCmd.IdentitiesOnly
	}

	paths := identityFiles
	if len(paths) == 0 && !identitiesOnly {
		paths = defaultSSHKeyPaths()
	}
	keySigners, keyPaths, keyFingerprints := loadSSHSigners(paths, opts)
	if identitiesOnly {
		if len(keySigners) == 0 {
			debugf(opts, "git: auth=ssh none user=%s identities_only", user)
			return nil, nil
		}
		debugf(opts, "git: auth=ssh keys=%d identities_only", len(keySigners))
		if len(keyFingerprints) > 0 {
			debugf(opts, "git: auth=ssh key_fingerprints=%s", strings.Join(keyFingerprints, ","))
		}
		return &gitssh.PublicKeysCallback{
			User: user,
			Callback: func() ([]ssh.Signer, error) {
				return keySigners, nil
			},
		}, nil
	}

	agentSigners := []ssh.Signer{}
	var agentFingerprints []string
	agent, _, err := sshagent.New()
	if err == nil {
		agentSigners, _ = agent.Signers()
		for _, signer := range agentSigners {
			agentFingerprints = append(agentFingerprints, ssh.FingerprintSHA256(signer.PublicKey()))
		}
	}

	if len(keySigners) == 0 && len(agentSigners) == 0 {
		debugf(opts, "git: auth=ssh none user=%s", user)
		return nil, nil
	}
	if len(keySigners) > 0 {
		debugf(opts, "git: auth=ssh keys=%d paths=%s", len(keySigners), strings.Join(keyPaths, ","))
		if len(keyFingerprints) > 0 {
			debugf(opts, "git: auth=ssh key_fingerprints=%s", strings.Join(keyFingerprints, ","))
		}
	}
	if len(agentSigners) > 0 {
		debugf(opts, "git: auth=ssh agent_keys=%d", len(agentSigners))
		if len(agentFingerprints) > 0 {
			debugf(opts, "git: auth=ssh agent_fingerprints=%s", strings.Join(agentFingerprints, ","))
		}
	}

	return &gitssh.PublicKeysCallback{
		User: user,
		Callback: func() ([]ssh.Signer, error) {
			combined := make([]ssh.Signer, 0, len(keySigners)+len(agentSigners))
			combined = append(combined, keySigners...)
			combined = append(combined, agentSigners...)
			return combined, nil
		},
	}, nil
}

func resolveSSHAliasEndpoint(ep *transport.Endpoint, overridePath string, opts LoadOptions) (*transport.Endpoint, error) {
	if ep == nil {
		return nil, nil
	}
	if ep.Protocol != "ssh" && ep.Protocol != "git+ssh" {
		return ep, nil
	}
	host := strings.Trim(ep.Host, "[]")
	if host == "" {
		return ep, nil
	}
	cfg := sshConfig(host, overridePath)
	changed := false
	if resolved := strings.TrimSpace(cfg.HostName); resolved != "" && resolved != host {
		if opts.Debug {
			debugf(opts, "git: ssh endpoint host alias=%s hostname=%s", host, resolved)
		}
		ep.Host = resolved
		changed = true
	}
	if resolvedPort := strings.TrimSpace(cfg.Port); resolvedPort != "" {
		port, err := strconv.Atoi(resolvedPort)
		if err == nil && port > 0 && ep.Port != port {
			if opts.Debug {
				debugf(opts, "git: ssh endpoint port alias=%s port=%d", host, port)
			}
			ep.Port = port
			changed = true
		}
	}
	if !changed && opts.Debug {
		debugf(opts, "git: ssh endpoint alias=%s (no hostname/port override)", host)
	}
	return ep, nil
}

func sshConfig(host string, overridePath string) sshHostConfig {
	cfgPath := overridePath
	if cfgPath == "" {
		cfgPath = os.Getenv("SSH_CONFIG")
	}
	if cfgPath != "" {
		f, err := os.Open(cfgPath)
		if err != nil {
			return sshHostConfig{Source: cfgPath}
		}
		defer f.Close()
		cfg, err := sshconfig.Decode(f)
		if err != nil {
			return sshHostConfig{Source: cfgPath}
		}
		user, _ := cfg.Get(host, "User")
		identityFiles, _ := cfg.GetAll(host, "IdentityFile")
		identitiesOnly, _ := cfg.Get(host, "IdentitiesOnly")
		hostName, _ := cfg.Get(host, "HostName")
		port, _ := cfg.Get(host, "Port")
		return sshHostConfig{
			User:           user,
			IdentityFiles:  identityFiles,
			IdentitiesOnly: strings.EqualFold(identitiesOnly, "yes"),
			HostName:       hostName,
			Port:           port,
			Source:         cfgPath,
		}
	}
	user := sshconfig.Get(host, "User")
	identityFiles := sshconfig.GetAll(host, "IdentityFile")
	identitiesOnly := sshconfig.Get(host, "IdentitiesOnly")
	hostName := sshconfig.Get(host, "HostName")
	port := sshconfig.Get(host, "Port")
	return sshHostConfig{
		User:           user,
		IdentityFiles:  identityFiles,
		IdentitiesOnly: strings.EqualFold(identitiesOnly, "yes"),
		HostName:       hostName,
		Port:           port,
		Source:         "default",
	}
}

func defaultSSHKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	base := filepath.Join(home, ".ssh")
	return []string{
		filepath.Join(base, "id_ed25519"),
		filepath.Join(base, "id_rsa"),
		filepath.Join(base, "id_ecdsa"),
		filepath.Join(base, "id_dsa"),
	}
}

func expandHome(pathValue string) string {
	if pathValue == "" {
		return ""
	}
	if strings.HasPrefix(pathValue, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return pathValue
		}
		if pathValue == "~" {
			return home
		}
		if strings.HasPrefix(pathValue, "~/") {
			return filepath.Join(home, pathValue[2:])
		}
	}
	return pathValue
}

func loadSSHSigners(paths []string, opts LoadOptions) ([]ssh.Signer, []string, []string) {
	var signers []ssh.Signer
	var used []string
	var fingerprints []string
	for _, keyPath := range paths {
		keyPath = expandHome(keyPath)
		if keyPath == "" {
			continue
		}
		if _, err := os.Stat(keyPath); err != nil {
			continue
		}
		data, err := os.ReadFile(keyPath)
		if err != nil {
			debugf(opts, "git: auth=ssh-key read failed path=%s", keyPath)
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			if _, ok := err.(*ssh.PassphraseMissingError); ok {
				debugf(opts, "git: auth=ssh-key passphrase required path=%s", keyPath)
			} else {
				debugf(opts, "git: auth=ssh-key parse failed path=%s", keyPath)
			}
			continue
		}
		signers = append(signers, signer)
		used = append(used, keyPath)
		fingerprints = append(fingerprints, ssh.FingerprintSHA256(signer.PublicKey()))
	}
	return signers, used, fingerprints
}

func parseGitSSHCommand() sshCommandOptions {
	raw := strings.TrimSpace(os.Getenv("GIT_SSH_COMMAND"))
	if raw == "" {
		return sshCommandOptions{}
	}
	args := splitShellArgs(raw)
	if len(args) == 0 {
		return sshCommandOptions{}
	}
	opts := sshCommandOptions{}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-i" && i+1 < len(args):
			opts.IdentityFiles = append(opts.IdentityFiles, args[i+1])
			i++
		case strings.HasPrefix(arg, "-i") && len(arg) > 2:
			opts.IdentityFiles = append(opts.IdentityFiles, arg[2:])
		case arg == "-F" && i+1 < len(args):
			opts.ConfigFile = args[i+1]
			i++
		case strings.HasPrefix(arg, "-F") && len(arg) > 2:
			opts.ConfigFile = arg[2:]
		case arg == "-l" && i+1 < len(args):
			opts.User = args[i+1]
			i++
		case strings.HasPrefix(arg, "-l") && len(arg) > 2:
			opts.User = arg[2:]
		case arg == "-o" && i+1 < len(args):
			applySSHOption(&opts, args[i+1])
			i++
		case strings.HasPrefix(arg, "-o") && len(arg) > 2:
			applySSHOption(&opts, arg[2:])
		}
	}
	return opts
}

func applySSHOption(opts *sshCommandOptions, raw string) {
	if raw == "" {
		return
	}
	parts := strings.SplitN(raw, "=", 2)
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	val := ""
	if len(parts) == 2 {
		val = strings.TrimSpace(parts[1])
	}
	switch key {
	case "user":
		if val != "" {
			opts.User = val
		}
	case "identityfile":
		if val != "" {
			opts.IdentityFiles = append(opts.IdentityFiles, val)
		}
	case "identitiesonly":
		if val == "" {
			return
		}
		boolVal := strings.EqualFold(val, "yes") || strings.EqualFold(val, "true") || val == "1"
		opts.IdentitiesOnly = &boolVal
	}
}

func splitShellArgs(input string) []string {
	var out []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if buf.Len() > 0 {
			out = append(out, buf.String())
			buf.Reset()
		}
	}

	for _, r := range input {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return out
}

func writeSourceMetadata(repoDir, url, ref, commit, pathValue, subtree string) error {
	data := SourceMetadata{
		URL:       url,
		Ref:       ref,
		Commit:    commit,
		Path:      pathValue,
		Subtree:   subtree,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	metaDir := filepath.Join(repoDir, ".swarmcp_sources")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(metaDir, hashKey(url+"|"+ref+"|"+pathValue)+".json")
	return os.WriteFile(path, encoded, 0o644)
}

func hashKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
