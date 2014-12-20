// interactive web user interface
package web

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"ngrok/client/assets"
	"ngrok/client/mvc"
	"ngrok/log"
	"ngrok/proto"
	"ngrok/util"
	"path"
	metrics "github.com/rcrowley/go-metrics"
	"os"
)

type WebView struct {
	log.Logger

	ctl mvc.Controller

	// messages sent over this broadcast are sent to all websocket connections
	wsMessages *util.Broadcast
}

func NewWebView(ctl mvc.Controller, addr string) *WebView {
	wv := &WebView{
		Logger:     log.NewPrefixLogger("view", "web"),
		wsMessages: util.NewBroadcast(),
		ctl:        ctl,
	}

	// for now, always redirect to the http view
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/http/in", 302)
	})

	// handle web socket connections
	http.HandleFunc("/_ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)

		if err != nil {
			http.Error(w, "Failed websocket upgrade", 400)
			wv.Warn("Failed websocket upgrade: %v", err)
			return
		}

		msgs := wv.wsMessages.Reg()
		defer wv.wsMessages.UnReg(msgs)
		for m := range msgs {
			err := conn.WriteMessage(websocket.TextMessage, m.([]byte))
			if err != nil {
				// connection is closed
				break
			}
		}
	})

	// serve static assets
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		buf, err := assets.Asset(path.Join("assets", "client", r.URL.Path[1:]))
		if err != nil {
			wv.Warn("Error serving static file: %s", err.Error())
			http.NotFound(w, r)
			return
		}
		w.Write(buf)
	})

	http.HandleFunc("/rest/exit", func(w http.ResponseWriter, r *http.Request) {
		os.Exit(1)
	})

	http.HandleFunc("/rest/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		j := make(map[string]interface{})
		j["tunnels"] = ctl.State().GetTunnels()
		j["client_version"] = ctl.State().GetClientVersion()
		j["server_version"] = ctl.State().GetServerVersion()
		j["connection_status"] = ctl.State().GetConnStatus()

		values := make(map[string]interface{})
		// Metrics
		meter,_ := ctl.State().GetConnectionMetrics()
		m := meter.Snapshot()
		values["count"] = m.Count()
		values["1m.rate"] = m.Rate1()
		values["5m.rate"] = m.Rate5()
		values["15m.rate"] = m.Rate15()
		values["mean.rate"] = m.RateMean()
		j["connection_metrics"] = values

		var counter metrics.Counter
		var histogram metrics.Histogram

		values = make(map[string]interface{})
		counter, histogram = ctl.State().GetBytesInMetrics()
		values["count"] = counter.Count()
		h := histogram.Snapshot()
		ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
		values["min"] = h.Min()
		values["max"] = h.Max()
		values["mean"] = h.Mean()
		values["stddev"] = h.StdDev()
		values["median"] = ps[0]
		values["75%"] = ps[1]
		values["95%"] = ps[2]
		values["99%"] = ps[3]
		values["99.9%"] = ps[4]
		j["bytes_in"] = values

		values = make(map[string]interface{})
		counter,histogram = ctl.State().GetBytesOutMetrics()
		values["count"] = counter.Count()
		h = histogram.Snapshot()
		ps = h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
		values["min"] = h.Min()
		values["max"] = h.Max()
		values["mean"] = h.Mean()
		values["stddev"] = h.StdDev()
		values["median"] = ps[0]
		values["75%"] = ps[1]
		values["95%"] = ps[2]
		values["99%"] = ps[3]
		values["99.9%"] = ps[4]
		j["bytes_out"] = values

		a, err := json.Marshal(j)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(a)
		return
	})

	wv.Info("Serving web interface on %s", addr)
	wv.ctl.Go(func() { http.ListenAndServe(addr, nil) })
	return wv
}

func (wv *WebView) NewHttpView(proto *proto.Http) *WebHttpView {
	return newWebHttpView(wv.ctl, wv, proto)
}

func (wv *WebView) Shutdown() {
}
