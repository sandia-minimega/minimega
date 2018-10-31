package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

var empty time.Time

func TestCalculateStats(t *testing.T) {
	start := time.Now().AddDate(0, 0, -5)
	window := start.Add((time.Nanosecond * -5))
	globalStats, counter := genResall(start, window)
	globalStats.calculateStats(window)
	if globalStats.NumNodes != 5 {
		t.Error("nodes used error counting nodes for time period, got", globalStats.NumNodes, " expected ", 5)
	}
	if globalStats.NumUsers != 4 {
		t.Error("users error counting users for time period, got", globalStats.NumUsers, " expected ", 4)
	}
	if globalStats.TotalExtensions != 5 {
		t.Error("error counting extensions for time period, got", globalStats.TotalExtensions, " expected ", 5)
	}
	if globalStats.TotalEarlyCancels != 2 {
		t.Error("early cancels error counting extensions for time period, got", globalStats.TotalEarlyCancels, " expected ", 2)
	}
	if globalStats.TotalDurationMinutes > (counter+(time.Second*30)) || globalStats.TotalDurationMinutes < (counter-(time.Second*30)) {
		t.Error("duration error counting duration for time period, got", globalStats.TotalDurationMinutes, " expected ", counter)
	}
}

func TestPrintStats(t *testing.T) {
	start := time.Now().AddDate(0, 0, -5)
	window := start.Add((time.Nanosecond * -5))
	globalStats, _ := genResall(start, window)
	globalStats.calculateStats(window)
	test := printStats(globalStats)
	if ok, err := testout(test); !ok {
		t.Error(err)
	}
}

func genRes(user string, start time.Time, end time.Time, actualend time.Time, id int, numnodes int, numext int) *ResData {
	var nodes []string
	var res *ResData
	for i := 0; i < numnodes; i++ {
		nodes = append(nodes, strconv.Itoa(i))
	}
	if actualend != empty {
		res = &ResData{user, start, end, actualend, actualend.Sub(start), id, nodes, numext}
	} else {
		res = &ResData{ResName: user, ResStart: start, ResEnd: end, ResId: id, Nodes: nodes, NumExtensions: numext}
	}
	return res
}

func genResall(start time.Time, window time.Time) (*Stats, time.Duration) {
	globalStats := Stats{}
	globalStats.Reservations = make(map[string][]*ResData)
	globalStats.NodesUsed = make(map[string]int)
	globalStats.Reservationsid = make(map[int]*ResData)
	var counter time.Duration
	nodecount := 1
	//test reservation start and end during window
	globalStats.Reservations["userA"] = []*ResData{genRes("userA", start, start.Add(time.Hour*24*6), start.Add(time.Hour*24*4), 1, nodecount, 0)}
	counter += (start.Add(time.Hour * 24 * 4).Sub(start)) * time.Duration(nodecount)
	//test reservation start and with no end during window
	globalStats.Reservations["userB"] = []*ResData{genRes("userB", start, start.Add(time.Hour*24*6), empty, 2, nodecount+1, 0)}
	counter += time.Now().Sub(window) * time.Duration(nodecount+1)
	//test reservation with start and end before window
	globalStats.Reservations["userC"] = []*ResData{genRes("userC", start.Add(time.Hour*24*-5), start.Add(time.Hour*24*1), start.Add(time.Hour*24*-4), 3, nodecount+2, 0)}
	//test reservation with start and no end during window differnet num of extends
	globalStats.Reservations["userD"] = []*ResData{genRes("userD", start, start.Add(time.Hour*24*10), empty, 4, nodecount+3, 3)}
	counter += time.Now().Sub(window) * time.Duration(nodecount+3)
	//test reservation with start before window and end during window differnet num of extends
	globalStats.Reservations["userE"] = []*ResData{genRes("userE", start.Add(time.Hour*24*-1), start.Add(time.Hour*24*6), start.Add(time.Hour*24*4), 5, nodecount+4, 2)}
	counter += (start.Add(time.Hour * 24 * 4).Sub(window)) * time.Duration(nodecount+4)
	return &globalStats, counter
}

func testout(test string) (bool, string) {
	control := []string{
		"------------------Global Statistics for all nodes------------------",
		"Total Users: 4",
		"Number of Nodes used: 5",
		"Total Number of Reservations: 4",
		"Total Number of Reservations Cancelled Early: 2",
		"Total Number of Extensions: 5",
		"Total Reservation Time: 07 Weeks 05 days 00 hours 00 minutes",
	}
	testarr := strings.Split(test, "\n")
	var b bytes.Buffer
	for i := 0; i < len(control); i++ {
		if control[i] != testarr[i] {
			fmt.Fprintf(&b, "expected: %v got %v \n", control[i], testarr[i])
		}
	}

	if b.Len() != 0 {
		return false, b.String()
	}
	return true, ""

}
