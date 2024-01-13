package web

import "sync"

type Hosts struct {
	sync.Mutex
	entries map[string]string
}

func NewHosts() *Hosts {
	return &Hosts{
		entries: make(map[string]string),
	}
}

func (h *Hosts) Set(key, ipv4, domain string) {
	h.Lock()
	defer h.Unlock()

	h.entries[key] = ipv4 + " " + domain
}

func (h *Hosts) Delete(key string) {
	h.Lock()
	defer h.Unlock()

	delete(h.entries, key)
}

func (h *Hosts) render() string {
	h.Lock()
	defer h.Unlock()

	var hosts string
	for _, entry := range h.entries {
		hosts += entry + "\n"
	}
	return hosts
}
