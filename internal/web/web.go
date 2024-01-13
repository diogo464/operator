package web

import "net/http"

func Start(forwards *Forwards, hosts *Hosts) error {

	http.HandleFunc("/hosts", func(w http.ResponseWriter, r *http.Request) {
		response := hosts.render()
		w.Write([]byte(response))
	})

	http.HandleFunc("/forward", func(w http.ResponseWriter, r *http.Request) {
		response := forwards.render()
		w.Write([]byte(response))
	})

	return http.ListenAndServe(":3000", nil)
}
