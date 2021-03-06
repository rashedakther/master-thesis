/*
    Maxime Piraux's master's thesis
    Copyright (C) 2017-2018  Maxime Piraux

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU Affero General Public License version 3
	as published by the Free Software Foundation.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU Affero General Public License for more details.

    You should have received a copy of the GNU Affero General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package main

import (
	"flag"
	"os"
	"sort"
	"github.com/mpiraux/master-thesis/scenarii"
	m "github.com/mpiraux/master-thesis"
	"bufio"
	"strings"
	"os/exec"
	"runtime"
	"path"
	"io/ioutil"
	"fmt"
	"sync"
	"encoding/json"
	"time"
)

func main() {
	hostsFilename := flag.String("hosts", "", "A tab-separated file containing hosts and the URLs used to request data to be sent.")
	scenarioName := flag.String("scenario", "", "A particular scenario to run. Run all of them if the parameter is missing.")
	outputFilename := flag.String("output", "", "The file to write the output to. Output to stdout if not set.")
	logsDirectory := flag.String("logs-directory", "/tmp", "Location of the logs.")
	netInterface := flag.String("interface", "", "The interface to listen to when capturing pcaps. Lets tcpdump decide if not set.")
	parallel := flag.Bool("parallel", false, "Runs each scenario against multiple hosts at the same time.")
	maxInstances := flag.Int("max-instances", 10, "Limits the number of parallel scenario runs.")
	debug := flag.Bool("debug", false, "Enables debugging information to be printed.")
	flag.Parse()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		println("No caller information")
		os.Exit(-1)
	}
	scenarioRunnerFilename := path.Join(path.Dir(filename), "scenario_runner.go")

	if *hostsFilename == "" {
		println("The hosts parameter is required")
		os.Exit(-1)
	}

	file, err := os.Open(*hostsFilename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scenariiInstances := scenarii.GetAllScenarii()

	var scenarioIds []string
	for scenarioId := range scenariiInstances {
		scenarioIds = append(scenarioIds, scenarioId)
	}
	sort.Strings(scenarioIds)

	if *scenarioName != "" && scenariiInstances[*scenarioName] == nil {
		println("Unknown scenario", *scenarioName)
	}

	var results Results
	result := make(chan *m.Trace)

	go func() {
		for t := range result {
			results = append(results, *t)
		}
	}()

	for _, id := range scenarioIds {
		if *scenarioName != "" && *scenarioName != id {
			continue
		}
		scenario := scenariiInstances[id]

		if !*parallel {
			*maxInstances = 1
		}
		semaphore := make(chan bool, *maxInstances)
		for i := 0; i < *maxInstances; i++ {
			semaphore <- true
		}
		wg := &sync.WaitGroup{}

		os.MkdirAll(path.Join(*logsDirectory, id), os.ModePerm)

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.Split(scanner.Text(), "\t")
			host, url := line[0], line[1]

			<-semaphore
			wg.Add(1)
			if *debug {
				fmt.Println("starting", scenario.Name(), "against", host)
			}

			go func() {
				defer func() { semaphore <- true }()
				defer wg.Done()

				outputFile, err := ioutil.TempFile("", "quic_tracker")
				if err != nil {
					println(err.Error())
					return
				}
				outputFile.Close()

				logFile, err := os.Create(path.Join(*logsDirectory, id, host))
				if err != nil {
					println(err.Error())
					return
				}
				defer logFile.Close()

				crashTrace := GetCrashTrace(scenario, host) // Prepare one just in case
				start := time.Now()

				args := []string{"run", scenarioRunnerFilename, "-host", host, "-url", url, "-scenario", id, "-interface", *netInterface, "-output", outputFile.Name()}
				if *debug {
					args = append(args, "-debug")
				}

				c := exec.Command("go", args...)
				c.Stdout = logFile
				c.Stderr = logFile
				err = c.Run()
				if err != nil {
					println(err.Error())
				}

				var trace m.Trace
				outputFile, err = os.Open(outputFile.Name())
				if err != nil {
					println(err)
				}
				defer outputFile.Close()
				defer os.Remove(outputFile.Name())

				err = json.NewDecoder(outputFile).Decode(&trace)
				if err != nil {
					println(err.Error())
					crashTrace.StartedAt = start.Unix()
					crashTrace.Duration = uint64(time.Now().Sub(start).Seconds() * 1000)
					result <- crashTrace
					return
				}
				result <- &trace
			}()
		}

		wg.Wait()
		file.Seek(0, 0)
	}
	close(result)

	sort.Sort(results)
	out, _ := json.Marshal(results)
	if *outputFilename != "" {
		outFile, err := os.Create(*outputFilename)
		defer outFile.Close()
		if err == nil {
			outFile.Write(out)
			return
		} else {
			println(err.Error())
		}
	}

	println(string(out))
}

func GetCrashTrace(scenario scenarii.Scenario, host string) *m.Trace {
	trace := m.NewTrace(scenario.Name(), scenario.Version(), host)
	trace.ErrorCode = 254
	return trace
}

type Results []m.Trace
func (a Results) Less(i, j int) bool {
	if a[i].Scenario == a[j].Scenario {
		return a[i].Host < a[j].Host
	}
	return a[i].Scenario < a[j].Scenario
}
func (a Results) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Results) Len() int           { return len(a) }