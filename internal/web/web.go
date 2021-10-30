package web

import (
	"fmt"
	"github.com/antigloss/go/logger"
	"github.com/flavio-fernandes/smokey/internal/manager"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var mgr *manager.Manager

var (
	epoch          = time.Unix(0, 0).Format(time.RFC1123)
	noCacheHeaders = map[string]string{
		"Expires":         epoch,
		"Cache-Control":   "no-cache, private, max-age=0",
		"Pragma":          "no-cache",
		"X-Accel-Expires": "0",
	}
	etagHeaders = []string{
		"ETag",
		"If-Modified-Since",
		"If-Match",
		"If-None-Match",
		"If-Range",
		"If-Unmodified-Since",
	}
)

func Start(manager *manager.Manager, listenPort string) {
	mgr = manager
	go webWorker(listenPort)
}

func webWorker(listenPort string) {
	http.HandleFunc("/", index)
	logger.Infof("Starting web server on port %s", listenPort)
	logger.Fatal(http.ListenAndServe(":"+listenPort, nil))
}

func noCache(w http.ResponseWriter, r *http.Request) {
	// Delete any ETag headers that may have been set
	for _, v := range etagHeaders {
		if r.Header.Get(v) != "" {
			r.Header.Del(v)
		}
	}

	// Set our NoCache headers
	for k, v := range noCacheHeaders {
		w.Header().Set(k, v)
	}
}

func managerState(w http.ResponseWriter, _ *http.Request) {
	response := mgr.CurrState()
	if response == nil {
		errorStr := "Unable to get state from manager"
		logger.Error(errorStr)
		http.Error(w, errorStr, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(response); err != nil {
		logger.Errorf("Failed sending response: %v", err)
	}
}

func managerQueryStatus(w http.ResponseWriter, r *http.Request) {
	mgr.CmdQueryStatus()
	managerState(w, r)
}

func managerStateWater(w http.ResponseWriter, _ *http.Request) {
	response := mgr.CurrStateWater()
	if response == nil {
		errorStr := "Unable to get state water from manager"
		logger.Error(errorStr)
		http.Error(w, errorStr, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=us-ascii")
	if _, err := w.Write(response); err != nil {
		logger.Errorf("Failed sending state water response: %v", err)
	}
}

func badRequest(w http.ResponseWriter, errorStr string) {
	logger.Error(errorStr)
	http.Error(w, errorStr, http.StatusBadRequest)
}

func noContent(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func lighton(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = r.ParseForm(); err != nil {
		badRequest(w, fmt.Sprintf("bad form for lighton: %v", err))
		return
	}
	autoOffSecsStr := r.FormValue("autoOffSecs")
	modeStr := r.FormValue("mode")
	colorStr := r.FormValue("color")

	autoOffSecs := manager.DefaultAutoOffSeconds
	if autoOffSecsStr != "" {
		v, err := strconv.ParseInt(autoOffSecsStr, 10, 32)
		if err != nil {
			badRequest(w, fmt.Sprintf("bad autoOffSecs for lighton: %v", err))
			return
		}
		autoOffSecs = int(v)
	}
	mode := manager.Crazy
	if modeStr != "" {
		mode, err = manager.LightModeVal(modeStr)
		if err != nil {
			badRequest(w, fmt.Sprintf("bad mode for lighton: %v", err))
			return
		}
	}
	mgr.CmdLightOn(autoOffSecs, mode, manager.LightColor(colorStr))
	noContent(w)
}

func lightoff(w http.ResponseWriter, _ *http.Request) {
	mgr.CmdLightOff()
	noContent(w)
}

func lightcolor(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = r.ParseForm(); err != nil {
		badRequest(w, fmt.Sprintf("bad form for lightcolor: %v", err))
		return
	}
	colorStr := r.FormValue("color")
	mgr.CmdLightColor(manager.LightColor(colorStr))
	noContent(w)
}

func lightdim(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = r.ParseForm(); err != nil {
		badRequest(w, fmt.Sprintf("bad form for lightdim: %v", err))
		return
	}
	dimStr := r.FormValue("dim")
	dim, err := strconv.ParseInt(dimStr, 10, 32)
	if err != nil {
		badRequest(w, fmt.Sprintf("bad value for lightdim %s: %v", dimStr, err))
		return
	}
	if dim < 0 || dim > 100 {
		badRequest(w, fmt.Sprintf("bad dim: %s. Should be between 0 and 100", dimStr))
		return
	}
	mgr.CmdLightDim(int(dim))
	noContent(w)
}

func diffuseron(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = r.ParseForm(); err != nil {
		badRequest(w, fmt.Sprintf("bad form for diffuseron: %v", err))
		return
	}
	autoOffSecsStr := r.FormValue("autoOffSecs")
	autoOffSecs := manager.DefaultAutoOffSeconds
	if autoOffSecsStr != "" {
		v, err := strconv.ParseInt(autoOffSecsStr, 10, 32)
		if err != nil {
			badRequest(w, fmt.Sprintf("bad autoOffSecs for diffuseron: %v", err))
			return
		}
		autoOffSecs = int(v)
	}
	mgr.CmdDiffuserOn(autoOffSecs)
	noContent(w)
}

func diffuseroff(w http.ResponseWriter, _ *http.Request) {
	mgr.CmdDiffuserOff()
	noContent(w)
}

var (
	getters = map[string]func(http.ResponseWriter, *http.Request){
		"/":       managerState,
		"/state":  managerState,
		"/status": managerState,
		"/query":  managerQueryStatus,
		"/water":  managerStateWater,
	}
	posters = map[string]func(http.ResponseWriter, *http.Request){
		"/inform":      http.NotFound,
		"/query":       managerQueryStatus,
		"/lighton":     lighton,
		"/lightcolor":  lightcolor,
		"/lightdim":    lightdim,
		"/lightoff":    lightoff,
		"/smokeon":     diffuseron,
		"/smokeoff":    diffuseroff,
		"/diffuseron":  diffuseron,
		"/diffuseroff": diffuseroff,
	}
	deleters = map[string]func(http.ResponseWriter, *http.Request){
		"/lighton":    lightoff,
		"/smokeon":    diffuseroff,
		"/diffuseron": diffuseroff,
	}
)

func index(w http.ResponseWriter, r *http.Request) {
	noCache(w, r)
	var haveHandler bool
	var handler func(http.ResponseWriter, *http.Request)
	if strings.ToLower(r.Method) == "get" {
		handler, haveHandler = getters[r.RequestURI]
	} else if strings.ToLower(r.Method) == "post" {
		handler, haveHandler = posters[r.RequestURI]
	} else if strings.ToLower(r.Method) == "delete" {
		handler, haveHandler = deleters[r.RequestURI]
	}
	if !haveHandler {
		handler = http.NotFound
	}
	logger.Infof("serving %s %s: hit %v", r.Method, r.RequestURI, haveHandler)
	handler(w, r)
}
