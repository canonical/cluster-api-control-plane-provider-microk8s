/*
Copyright 2022.

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

package controllers

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

type SortByCreationTimestamp []clusterv1.Machine

func (it SortByCreationTimestamp) Len() int {
	return len(it)
}
func (it SortByCreationTimestamp) Less(i, j int) bool {
	return it[j].GetCreationTimestamp().After(it[i].GetCreationTimestamp().Time)
}
func (it SortByCreationTimestamp) Swap(i, j int) {
	it[i], it[j] = it[j], it[i]
}
