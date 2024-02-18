// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

const defaultConfigPath = "/config"

// set up a filesystem watcher for the mounted files
// right now just the configMap
// reboot the app whenever there is an update via the returned stopCh.
func initializeWatch(path string, log logrus.FieldLogger) (*fsnotify.Watcher, error) {
	watch, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalln("fail to setup config watcher")
	}
	go func() {
		for {
			select {
			case err := <-watch.Errors:
				log.Warningf("watcher receives err: %v\n", err)
			case event := <-watch.Events:
				if event.Op != fsnotify.Chmod {
					log.Fatalf("restarting contour because received event %v on file %s\n", event.Op.String(), path)
				} else {
					log.Printf("watcher receives %s on the mounted file %s\n", event.Op.String(), event.Name)
				}
			}
		}
	}()

	if err := watch.Add(path); err != nil {
		log.Fatalf("fail to watch contour config file %s\n", path)
	}
	return watch, err
}
