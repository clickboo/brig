package endpoints

import (
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/csrf"
	"github.com/phogolabs/parcello"
	log "github.com/sirupsen/logrus"

	// Include static resources:
	_ "github.com/sahib/brig/gateway/templates"
)

// IndexHandler implements http.Handler.
// It serves index.html from either file or memory.
type IndexHandler struct {
	*State
}

// NewIndexHandler returns a new IndexHandler.
func NewIndexHandler(s *State) *IndexHandler {
	return &IndexHandler{State: s}
}

func (ih *IndexHandler) loadTemplateData() (io.ReadCloser, error) {
	if ih.cfg.Bool("ui.debug_mode") {
		return os.Open("./gateway/templates/index.html")
	}

	mgr := parcello.ManagerAt("/")
	return mgr.Open("index.html")
}

func (ih *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fd, err := ih.loadTemplateData()
	if err != nil {
		jsonifyErrf(w, http.StatusInternalServerError, "no index.html")
		return
	}

	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		jsonifyErrf(w, http.StatusInternalServerError, "could not load template: %v", err)
		return
	}

	t, err := template.New("index").Parse(string(data))
	if err != nil {
		log.Errorf("could not parse template: %v", err)
		jsonifyErrf(w, http.StatusInternalServerError, "template contains errors")
		return
	}

	wsScheme := "ws://"
	if r.TLS != nil {
		wsScheme = "wss://"
	}

	httpScheme := "http://"
	if r.TLS != nil {
		httpScheme = "https://"
	}

	err = t.Execute(w, map[string]interface{}{
		"csrfToken": csrf.Token(r),
		"wsAddr":    wsScheme + r.Host + "/events",
		"httpAddr":  httpScheme + r.Host,
	})

	if err != nil {
		jsonifyErrf(w, http.StatusInternalServerError, "could not execute template")
		return
	}
}
