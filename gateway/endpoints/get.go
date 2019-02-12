package endpoints

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/sahib/brig/catfs"
	ie "github.com/sahib/brig/catfs/errors"
	"github.com/sahib/brig/catfs/mio"
	"github.com/sahib/brig/util"
)

// GetHandler implements http.Handler
type GetHandler struct {
	*State
}

// NewGetHandler returns a new GetHandler
func NewGetHandler(s *State) *GetHandler {
	return &GetHandler{State: s}
}

func mimeTypeFromStream(stream mio.Stream) (io.ReadSeeker, string) {
	hdr, newStream, err := util.PeekHeader(stream, 512)
	if err != nil {
		return stream, "application/octet-stream"
	}

	return newStream, http.DetectContentType(hdr)
}

// setContentDisposition sets the Content-Disposition header, based on
// the content we are serving. It tells a browser if it should open
// a save dialog or display it inline (and how)
func setContentDisposition(info *catfs.StatInfo, hdr http.Header, dispoType string) {
	basename := path.Base(info.Path)
	if info.IsDir {
		if basename == "/" {
			basename = "root"
		}

		basename += ".tar"
	}

	hdr.Set(
		"Content-Disposition",
		fmt.Sprintf(
			"%s; filename*=UTF-8''%s",
			dispoType,
			url.QueryEscape(basename),
		),
	)
}

func (gh *GetHandler) checkBasicAuth(nodePath string, w http.ResponseWriter, r *http.Request) bool {
	name, pass, ok := r.BasicAuth()

	// No basic auth sent. If a browser send the request: ask him to
	// show a user/password form that gives a chance to change that.
	if !ok {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"brig gateway\"")
		return false
	}

	// Check is the basic auth credentials are valid.
	user, err := gh.userDb.Get(name)
	if err != nil {
		return false
	}

	isValid, err := user.CheckPassword(pass)
	if !isValid {
		if err != nil {
			log.Warningf("get: failed to check password: %v", err)
		}

		return false
	}

	// Check again if this user has access to the path:
	if !gh.validatePathForUser(nodePath, user, w, r) {
		return false
	}

	return true
}

func (gh *GetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the file nodePath including the leading slash:
	fullURL := r.URL.EscapedPath()
	nodePath, err := url.PathUnescape(fullURL[4:])
	if nodePath == "" {
		nodePath = "/"
	}

	if err != nil {
		log.Debugf("received malformed url: %s", fullURL)
		http.Error(w, "malformed url", http.StatusBadRequest)
		return
	}

	if gh.cfg.Bool("auth.enabled") {
		// validatePath will check if the user is actually logged in
		// and may access the path in question. The login could come
		// from a previous login to the UI (the /get endpoint could be used separately)
		if !gh.validatePath(nodePath, w, r) {
			if !gh.checkBasicAuth(nodePath, w, r) {
				http.Error(w, "not authorized", http.StatusUnauthorized)
				return
			}
		}

		//  All good. Proceed with the content.
	}

	info, err := gh.fs.Stat(nodePath)
	if err != nil {
		// Handle a bad nodePath more explicit:
		if ie.IsNoSuchFileError(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		log.Errorf("gateway: failed to stat %s: %v", nodePath, err)
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}

	hdr := w.Header()
	hdr.Set("ETag", info.ContentHash.B58String())
	hdr.Set("Last-Modified", info.ModTime.Format(http.TimeFormat))

	if info.IsDir {
		params := r.URL.Query()
		includes := params["include"]

		filter := func(info *catfs.StatInfo) bool {
			if len(includes) == 0 {
				return true
			}

			for _, include := range includes {
				if strings.HasPrefix(info.Path, include) {
					return true
				}
			}

			return false
		}

		setContentDisposition(info, hdr, "attachment")
		if err := gh.fs.Tar(nodePath, w, filter); err != nil {
			log.Errorf("gateway: failed to stream %s: %v", nodePath, err)
			http.Error(w, "failed to stream", http.StatusInternalServerError)
			return
		}
	} else {
		stream, err := gh.fs.Cat(nodePath)
		if err != nil {
			log.Errorf("gateway: failed to stream %s: %v", nodePath, err)
			http.Error(w, "failed to stream", http.StatusInternalServerError)
			return
		}

		prefixStream, mimeType := mimeTypeFromStream(stream)
		hdr.Set("Content-Type", mimeType)
		hdr.Set("Content-Length", strconv.FormatUint(info.Size, 10))

		isDirectDownload := r.URL.Query().Get("direct") == "yes"

		// Set the content disposition to inline if it looks like something viewable.
		if mimeType == "application/octet-stream" || isDirectDownload {
			setContentDisposition(info, hdr, "attachment")
		} else {
			setContentDisposition(info, hdr, "inline")
		}

		http.ServeContent(w, r, path.Base(info.Path), info.ModTime, prefixStream)
	}
}
