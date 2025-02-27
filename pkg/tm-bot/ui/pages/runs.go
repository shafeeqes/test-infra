// Copyright 2019 Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pages

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	argov1alpha1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/test-infra/pkg/apis/testmachinery/v1beta1"
	"github.com/gardener/test-infra/pkg/common"
	tmetadata "github.com/gardener/test-infra/pkg/testmachinery/metadata"
	"github.com/gardener/test-infra/pkg/testrunner"
	"github.com/gardener/test-infra/pkg/tm-bot/ui/pages/pagination"
	"github.com/gardener/test-infra/pkg/util"
)

type testrunItem struct {
	testrun   *v1beta1.Testrun
	ID        string
	Namespace string
	RunID     string
	Phase     IconWithTooltip
	StartTime string
	Duration  string
	Progress  string

	Dimension       string
	Retries         string
	PreviousAttempt string

	ArgoURL    string
	GrafanaURL string
}

type rungroupItem struct {
	testruns  []*v1beta1.Testrun
	phase     argov1alpha1.WorkflowPhase
	startTime *metav1.Time
	completed int

	DisplayName string
	Name        string
	StartTime   string
	State       string
	Phase       IconWithTooltip
}

func NewTestrunsPage(p *Page) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		defer ctx.Done()

		var (
			rgName   string
			listOpts = []client.ListOption{}
		)
		if rg, rgOk := r.URL.Query()[common.DashboardExecutionGroupParameter]; rgOk {
			rgName = rg[0]
			listOpts = append(listOpts, client.MatchingLabels(map[string]string{common.LabelTestrunExecutionGroup: rgName}))
		}

		runs := &v1beta1.TestrunList{}
		if err := p.runs.GetClient().List(ctx, runs, listOpts...); err != nil {
			p.log.Error(err, "unable to fetch testruns")
			http.Redirect(w, r, "/404", http.StatusTemporaryRedirect)
			return
		}

		argoHostURL, _ := testrunner.GetArgoHost(ctx, p.runs.GetClient())
		grafanaHostURL, _ := testrunner.GetGrafanaHost(ctx, p.runs.GetClient())

		testrunsList := make(testrunItemList, len(runs.Items))
		runsList := make(rungroupItemList, 0)
		for i, tr := range runs.Items {
			testrun := tr

			if rgName == "" {
				runsList.Add(&testrun)
			}
			metadata := tmetadata.FromTestrun(&tr)
			startTime := ""
			if tr.Status.StartTime != nil {
				startTime = tr.Status.StartTime.Format(time.RFC822)
			}

			d := time.Duration(tr.Status.Duration) * time.Second
			if tr.Status.Duration == 0 && !tr.Status.StartTime.IsZero() {
				d = time.Since(tr.Status.StartTime.Time)
				d = d / time.Second * time.Second // remove unnecessary milliseconds
			}

			testrunsList[i] = testrunItem{
				testrun:   &testrun,
				ID:        tr.GetName(),
				Namespace: tr.GetNamespace(),
				Phase:     RunPhaseIcon(tr.Status.Phase),
				StartTime: startTime,
				Duration:  d.String(),
				Progress:  util.TestrunProgress(&tr),
				Dimension: metadata.GetDimensionFromMetadata("/"),
			}
			if retries, ok := tr.Annotations[common.AnnotationRetries]; ok {
				testrunsList[i].Retries = retries
			}
			if prevAttempt, ok := tr.Annotations[common.AnnotationPreviousAttempt]; ok {
				testrunsList[i].PreviousAttempt = prevAttempt
			}
			if runID, ok := tr.Labels[common.LabelTestrunExecutionGroup]; ok {
				testrunsList[i].RunID = runID
			}
			if argoHostURL != "" {
				testrunsList[i].ArgoURL = testrunner.GetArgoURLFromHost(argoHostURL, &tr)
			}
			if grafanaHostURL != "" {
				testrunsList[i].GrafanaURL = testrunner.GetGrafanaURLFromHostForWorkflow(grafanaHostURL, tr.Status.Workflow)
			}
		}

		paginatedTestruns, pages := pagination.SliceFromValues(testrunsList, r.URL.Query())
		sort.Sort(runsList)
		params := map[string]interface{}{
			"rungroup":   rgName,
			"tests":      paginatedTestruns,
			"pages":      pages,
			"rungroups":  runsList,
			"pagination": false,
		}

		if len(runsList) > 6 {
			params["rungroups"] = runsList[:6]
		}

		p.handleSimplePage("testruns.html", params)(w, r)
	}
}

type rungroupItemList []rungroupItem

func (l rungroupItemList) Len() int      { return len(l) }
func (l rungroupItemList) Swap(a, b int) { l[a], l[b] = l[b], l[a] }
func (l *rungroupItemList) Add(tr *v1beta1.Testrun) {
	list := *l
	runId, ok := tr.GetLabels()[common.LabelTestrunExecutionGroup]
	if !ok {
		return
	}

	isCompleted := 0
	if util.CompletedRun(tr.Status.Phase) {
		isCompleted = 1
	}

	for i, run := range list {
		if run.Name == runId {
			list[i].testruns = append(run.testruns, tr)
			list[i].phase = mergePhases(run.phase, tr.Status.Phase)
			list[i].Phase = RunPhaseIcon(list[i].phase)
			list[i].completed = list[i].completed + isCompleted
			list[i].State = fmt.Sprintf("%d/%d Testruns are completed", list[i].completed, len(list[i].testruns))
			return
		}
	}

	startTime := &metav1.Time{}
	if tr.Status.StartTime != nil {
		startTime = tr.Status.StartTime
	}

	*l = append(*l, rungroupItem{
		testruns:    []*v1beta1.Testrun{tr},
		phase:       tr.Status.Phase,
		startTime:   tr.Status.StartTime,
		completed:   1,
		DisplayName: testgroupDisplayName(tr),
		Name:        runId,
		StartTime:   startTime.Format(time.RFC822),
		State:       fmt.Sprintf("%d/%d Testruns are completed", isCompleted, 1),
		Phase:       RunPhaseIcon(util.TestrunStatusPhase(tr)),
	})
}
func (l rungroupItemList) Less(a, b int) bool {
	if l[a].phase != l[b].phase {
		if l[a].phase == v1beta1.RunPhaseRunning {
			return true
		}
		if l[b].phase == v1beta1.RunPhaseRunning {
			return false
		}

		if l[a].phase == v1beta1.RunPhaseInit {
			return false
		}
		if l[b].phase == v1beta1.RunPhaseInit {
			return true
		}
	}
	if l[a].startTime == nil || l[b].startTime == nil {
		return true
	}
	return l[b].startTime.Before(l[a].startTime)
}

func mergePhases(a, b argov1alpha1.WorkflowPhase) argov1alpha1.WorkflowPhase {
	if a == v1beta1.RunPhaseRunning || b == v1beta1.RunPhaseRunning {
		return v1beta1.RunPhaseRunning
	}
	if a == v1beta1.RunPhaseFailed || b == v1beta1.RunPhaseFailed {
		return v1beta1.RunPhaseFailed
	}
	if a == v1beta1.RunPhaseError || b == v1beta1.RunPhaseError {
		return v1beta1.RunPhaseError
	}
	if a == v1beta1.RunPhaseTimeout || b == v1beta1.RunPhaseTimeout {
		return v1beta1.RunPhaseTimeout
	}
	return a
}

func testgroupDisplayName(tr *v1beta1.Testrun) string {
	displayName, ok := tr.GetAnnotations()[common.AnnotationLandscape]
	if !ok {
		return "Unknown"
	}

	if gp, ok := tr.GetAnnotations()[common.AnnotationGroupPurpose]; ok {
		displayName = fmt.Sprintf("%s - %s", displayName, gp)
	}
	return displayName
}
