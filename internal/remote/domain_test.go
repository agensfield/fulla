package remote

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestServerRejectsFutureSyncDomain(t *testing.T) {
	for _, version := range []int{CurrentProtocol, PreviousProtocol} {
		for _, phase := range []string{"before-authentication", "authenticated-session"} {
			t.Run(strconv.Itoa(version)+"/"+phase, func(t *testing.T) {
				left, right := paired(t)
				peer, err := left.Peer("other")
				if err != nil {
					t.Fatal(err)
				}
				c := session(t, right, version)
				if _, err := c.Hello(); err != nil {
					t.Fatal(err)
				}
				if phase == "authenticated-session" {
					if err := c.Authenticate(left, peer); err != nil {
						t.Fatal(err)
					}
				}
				newer := right.Meta
				newer.Domains = map[string]int{}
				for domain, value := range right.Meta.Domains {
					newer.Domains[domain] = value
				}
				newer.Domains["sync"] = 2
				data, err := json.Marshal(newer)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(right.Root, ".fulla/store.json", data); err != nil {
					t.Fatal(err)
				}
				// A fresh public discovery session still works with the future
				// sync domain, without authorizing any sync operation.
				discovery := session(t, right, version)
				if _, err := discovery.Hello(); err != nil {
					t.Fatal(err)
				}
				if err := discovery.Close(); err != nil {
					t.Fatal(err)
				}
				beforeLeft, beforeRight := storeDigest(t, left), storeDigest(t, right)
				if phase == "authenticated-session" {
					_, err = c.Inventory()
				} else {
					err = c.Authenticate(left, peer)
				}
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
					t.Fatal("future sync domain accepted by server", err)
				}
				if !reflect.DeepEqual(beforeLeft, storeDigest(t, left)) || !reflect.DeepEqual(beforeRight, storeDigest(t, right)) {
					t.Fatal("unsupported sync changed store files")
				}
			})
		}
	}
}
