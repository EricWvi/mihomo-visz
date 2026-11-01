package collector

import (
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
	"mihomo-visz/internal/mihomo"
	"mihomo-visz/internal/store"
)

func isInner(c mihomo.Connection) bool { return strings.EqualFold(c.Metadata.Type, "inner") }
func domain(m mihomo.Metadata) (string, string) {
	host := m.SniffHost
	if host == "" {
		host = m.Host
	}
	if host == "" {
		host = m.DestinationIP
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "未知地址", "未识别域名"
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return host, "未识别域名"
	}
	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		host = ascii
	}
	root, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host, host
	}
	return host, root
}
func node(chains []string, proxies map[string]mihomo.Proxy) string {
	for _, name := range chains {
		p, ok := proxies[name]
		if !ok {
			continue
		}
		switch strings.ToLower(p.Type) {
		case "selector", "urltest", "fallback", "loadbalance", "relay", "smart":
			continue
		}
		return name
	}
	return "未识别节点"
}

// hourStart floors the local wall clock, including time zones with a half-hour
// offset. Distinct occurrences of a repeated DST hour retain distinct instants.
func hourStart(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return t.Add(-time.Duration(t.Minute())*time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
}

type portion struct{ hour, upload, download int64 }

func split(from, to time.Time, up, down int64, loc *time.Location) []portion {
	if !to.After(from) || up < 0 || down < 0 {
		return nil
	}
	out := []portion{}
	remainingUp, remainingDown := up, down
	for cursor := from; cursor.Before(to); {
		h := hourStart(cursor, loc)
		end := h.Add(time.Hour)
		if end.After(to) {
			end = to
		}
		partUp, partDown := remainingUp, remainingDown
		if end.Before(to) {
			ratio := float64(end.Sub(cursor)) / float64(to.Sub(cursor))
			partUp = int64(float64(remainingUp) * ratio)
			partDown = int64(float64(remainingDown) * ratio)
		}
		out = append(out, portion{h.Unix(), partUp, partDown})
		remainingUp -= partUp
		remainingDown -= partDown
		cursor = end
	}
	return out
}

// diff attributes observed increments to sample intervals. It cannot recover
// short-lived connections absent from both snapshots or final unsampled bytes.
func diff(prev, current mihomo.Snapshot, from, to time.Time, proxies map[string]mihomo.Proxy, loc *time.Location) ([]store.Usage, []store.Observation) {
	previous := make(map[string]mihomo.Connection, len(prev.Connections))
	for _, c := range prev.Connections {
		previous[c.ID] = c
	}
	usage := []store.Usage{}
	seen := map[string]bool{}
	for _, c := range current.Connections {
		if c.ID == "" || seen[c.ID] || isInner(c) || c.Upload < 0 || c.Download < 0 {
			continue
		}
		seen[c.ID] = true
		up, down := c.Upload, c.Download
		start := from
		if p, ok := previous[c.ID]; ok {
			if isInner(p) || !p.Start.Equal(c.Start) || up < p.Upload || down < p.Download {
				continue
			}
			up -= p.Upload
			down -= p.Download
		} else {
			// A newly observed but older connection has an unknowable historical total.
			if c.Start.Before(from) || !c.Start.Before(to) {
				continue
			}
			start = c.Start
		}
		host, root := domain(c.Metadata)
		n := node(c.Chains, proxies)
		for _, p := range split(start, to, up, down, loc) {
			if p.upload > 0 || p.download > 0 {
				usage = append(usage, store.Usage{Hour: p.hour, Node: n, Host: host, Domain: root, Upload: p.upload, Download: p.download})
			}
		}
	}
	observations := []store.Observation{}
	for _, p := range split(from, to, current.UploadTotal-prev.UploadTotal, current.DownloadTotal-prev.DownloadTotal, loc) {
		observations = append(observations, store.Observation{Hour: p.hour, Upload: p.upload, Download: p.download})
	}
	return usage, observations
}
