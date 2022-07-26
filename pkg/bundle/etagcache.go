/*
Copyright 2022 The cert-manager Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bundle

import "sync"

// etagCache holds the responses to HTTPS bundle sources, with their etags and date.
type etagCache struct {
	cache map[string]dataWithEtag
	sync.RWMutex
}

type dataWithEtag struct {
	etag, date string
	data       []byte
}

func newEtagCache() *etagCache {
	return &etagCache{
		cache: make(map[string]dataWithEtag),
	}
}
