// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// readNetStats reads the tx or rx bytes for the given interface
func readNetStats(tap, name string) (int, error) {
	d, err := ioutil.ReadFile(fmt.Sprintf("/sys/class/net/%v/statistics/%v", tap, name))
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(string(d)))
}

func (b Bridges) updateBandwidthStats() {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, br := range b.bridges {
		for _, tap := range br.taps {
			if tap.Defunct {
				continue
			}

			rx, err := readNetStats(tap.Name, "rx_bytes")
			tx, err2 := readNetStats(tap.Name, "tx_bytes")
			if err != nil || err2 != nil {
				log.Debug("rx read err: %v, tx read err: %v", err, err2)
				continue
			}

			// Note: we swap the Rx and Tx bytes here because Rx on the host is
			// Tx on the VM and vice versa.
			tap.stats = append(tap.stats, tapStat{
				t:       time.Now(),
				RxBytes: tx,
				TxBytes: rx,
			})

			// truncate to 10 most recent results
			if len(tap.stats) > 10 {
				tap.stats = tap.stats[len(tap.stats)-10:]
			}
		}
	}
}

// BandwidthStats computes the sum of the rates of MB received and transmitted
// across all taps on the given bridge.
func (b Bridges) BandwidthStats() (float64, float64) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	var rxRate, txRate float64

	for _, br := range b.bridges {
		for _, tap := range br.taps {
			if !tap.Defunct {
				rx, tx := tap.BandwidthStats()

				rxRate += rx
				txRate += tx
			}
		}
	}

	return rxRate, txRate
}

// BandwidthStats computes the average rate of MB received and transmitted on
// the given tap over the 10 previous 5 second intervals. Returns the received
// and transmitted rates, in MBps.
func (t Tap) BandwidthStats() (float64, float64) {
	if len(t.stats) < 2 {
		return 0, 0
	}

	var rxRate, txRate float64

	n := float64(len(t.stats) - 1)

	// compute weighted moving average of history
	for i := range t.stats {
		if i+1 < len(t.stats) {
			rx := float64(t.stats[i+1].RxBytes - t.stats[i].RxBytes)
			tx := float64(t.stats[i+1].TxBytes - t.stats[i].TxBytes)
			d := float64(t.stats[i+1].t.Sub(t.stats[i].t).Seconds())

			// convert raw byte count to MB/s
			rxRate += (n - float64(i)) * (rx / float64(1<<20) / d)
			txRate += (n - float64(i)) * (tx / float64(1<<20) / d)
		}
	}

	rxRate /= n * (n + 1) / 2
	txRate /= n * (n + 1) / 2

	return rxRate, txRate
}
