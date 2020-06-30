package resolver

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/nat"
	"net"
	"regexp"
	"sort"
	"time"
)

type Resolver struct {
	lastSubnets []string
	expire      map[string]time.Time
	logger      zerolog.Logger
	nat         nat.NAT
	stopCh      chan struct{}
	stoppedCh   chan struct{}
}

var ipv4Regexp = regexp.MustCompile("\\A\\d+\\.\\d+\\.\\d+\\.\\d+(/\\d+)?\\z")

func New(logger zerolog.Logger, nat nat.NAT) *Resolver {
	return &Resolver{
		logger:    logger,
		nat:       nat,
		expire:    map[string]time.Time{},
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

func (r *Resolver) Stop() {
	close(r.stopCh)
	<-r.stoppedCh
}

func (r *Resolver) KeepUpdate(interval time.Duration, targets []string) {
	if err := r.update(targets); err != nil {
		r.logger.Warn().Err(err).Msg("Failed to update subnets")
	}

	tick := time.Tick(interval)
	for {
		select {
		case <-tick:
		case <-r.stopCh:
			close(r.stoppedCh)
			return
		}

		if err := r.update(targets); err != nil {
			r.logger.Warn().Err(err).Msg("Failed to update subnets")
		}
	}
}

func (r *Resolver) update(targets []string) error {
	r.logger.Debug().Msg("Updating subnets")

	subnetsMap := map[string]struct{}{}

	for _, target := range targets {
		if ipv4Regexp.MatchString(target) {
			subnetsMap[target] = struct{}{}
		} else {
			ips, err := net.LookupIP(target)
			if err != nil {
				return err
			}
			for _, ip := range ips {
				ipv4 := ip.To4()
				if ipv4 == nil {
					continue
				}
				subnetsMap[fmt.Sprintf("%s/32", ipv4.String())] = struct{}{}
			}
		}
	}

	// update expire time
	expire := time.Now().Add(time.Hour)
	for subnet := range subnetsMap {
		r.expire[subnet] = expire
	}

	// add not-expired subnets
	// delete expired subnets
	now := time.Now()
	for subnet, expireAt := range r.expire {
		if expireAt.After(now) {
			subnetsMap[subnet] = struct{}{}
		} else {
			delete(r.expire, subnet)
		}
	}

	var subnets []string
	for subnet := range subnetsMap {
		subnets = append(subnets, subnet)
	}
	sort.Strings(subnets)

	if r.areSubnetsUpdated(subnets) {
		if err := r.nat.RedirectSubnets(subnets); err != nil {
			return err
		}
	}

	r.lastSubnets = subnets

	return nil
}

func (r *Resolver) areSubnetsUpdated(subnets []string) bool {
	if len(r.lastSubnets) != len(subnets) {
		return true
	}

	for i, subnet := range subnets {
		if r.lastSubnets[i] != subnet {
			return true
		}
	}

	return false
}
