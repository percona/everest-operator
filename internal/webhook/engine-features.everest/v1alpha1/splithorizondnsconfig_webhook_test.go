// // everest-operator
// // Copyright (C) 2022 Percona LLC
// //
// // Licensed under the Apache License, Version 2.0 (the "License");
// // you may not use this file except in compliance with the License.
// // You may obtain a copy of the License at
// //
// // http://www.apache.org/licenses/LICENSE-2.0
// //
// // Unless required by applicable law or agreed to in writing, software
// // distributed under the License is distributed on an "AS IS" BASIS,
// // WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// // See the License for the specific language governing permissions and
// // limitations under the License.

// everest
// Copyright (C) 2025 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/engine-features.everest/v1alpha1"
	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
)

const (
	shdcName             = "my-shdc"
	shdcNamespace        = "default"
	shdcBaseDomainSuffix = "example.com"
	certSecretName       = "my-tls-secret" //nolint:gosec
	caCertFileBase64     = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURGRENDQWZ5Z0F3SUJBZ0lVUGJ1QWNmUTlqQldLZXI3dU9ka21hRnpqVlNZd0RRWUpLb1pJaHZjTkFRRUwKQlFBd0lqRU9NQXdHQTFVRUNoTUZVRk5OUkVJeEVEQU9CZ05WQkFNVEIxSnZiM1FnUTBFd0hoY05NalV4TURBNQpNVEV3T1RBd1doY05NekF4TURBNE1URXdPVEF3V2pBaU1RNHdEQVlEVlFRS0V3VlFVMDFFUWpFUU1BNEdBMVVFCkF4TUhVbTl2ZENCRFFUQ0NBU0l3RFFZSktvWklodmNOQVFFQkJRQURnZ0VQQURDQ0FRb0NnZ0VCQUxHa1pQMlMKR3JhRERzcmZZWGd2bmk2UkdRb0t0SFNBaFNhMjR6QjJRMWpxZzh0QW16TnZINENaN0QrN2tnNUdhb1RMUkpmeApSNTRyV1kzM2k4K2I2U0F1WTNsaWRLWTVuVnZRSmtBSDNHWk5ZbkJDYkVtWllWZEFNeUN4YmIyTjNmbWFsak1SCkxsVkpVNWtpdjlYQ1pPUVEyRDFWNXRsbURHclR5aXd5RUdOeThodjlwK0tRbXdiUU5vcmxDVWw5OEEzdEw3UHUKeWxVWFk4M0ZjeUZHNzU2VDZHQTBmQndTUHJOVzBvL1Iyejl6WHZjblVEYTF2a21qR2haT3R5WjNhT1RYdnEvMgpHdklzM2N4Q0t4Uy9kVyszTFR0WWtuaHAyUkZuWllZNUg0Uzh1bTBiUmNoQ0swZWl3SkpENzNNS1ZOMWRUVm5NCmVnT0o5Y2Qwc0NiL05aY0NBd0VBQWFOQ01FQXdEZ1lEVlIwUEFRSC9CQVFEQWdFR01BOEdBMVVkRXdFQi93UUYKTUFNQkFmOHdIUVlEVlIwT0JCWUVGSG5QMU11Q3ZsVmtYWHFCdkVad0t3ZXQxK2RwTUEwR0NTcUdTSWIzRFFFQgpDd1VBQTRJQkFRQmpQZ1FjVUVZOWtOekJQTWhTN2l4V0xMMDBnVFNtaTdqVlpJNEtRVzB3NkVKYVVyN0ppemVwClNPNW1ZekRyRWlQenZUbFlKS2V3UXdVUzVKZHQwVW5Ld2JQSmljaXp2UDNBT3gvT09LZzgwYUkyeFlYVWl2engKeCt6ZzI1ZThxaGxhdXpCOEUrdklHNFowd3FsS1A1dHhsRENmU3lrNkpFdUJzVHdCZm15d1dwWWRmYTRQTEw0TwpvT3FYYXcrQkpWZTFZeExHY1poQkNmSHVoM2E2c2FndU1LanNKS0pONjVnS0tOV0lkNm5Bb2pxRkNhSVczQ0xxCkU1dC9ZWldFaVIveFRoTGk0VTluUksrRUtxeDNVSlNjSlVkKzNlMCtibzgvYXYyTlJPdFNiSmRpNEJKSWN0T0MKOWgwZmZHaUlhT3ZNdHUrTGlJTHdCSnRxZisra1libDcKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	certFileBase64       = caCertFileBase64
	keyFileBase64        = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb2dJQkFBS0NBUUVBc2FSay9aSWF0b01PeXQ5aGVDK2VMcEVaQ2dxMGRJQ0ZKcmJqTUhaRFdPcUR5MENiCk0yOGZnSm5zUDd1U0RrWnFoTXRFbC9GSG5pdFpqZmVMejV2cElDNWplV0owcGptZFc5QW1RQWZjWmsxaWNFSnMKU1psaFYwQXpJTEZ0dlkzZCtacVdNeEV1VlVsVG1TSy8xY0prNUJEWVBWWG0yV1lNYXRQS0xESVFZM0x5Ry8ybgo0cENiQnRBMml1VUpTWDN3RGUwdnMrN0tWUmRqemNWeklVYnZucFBvWURSOEhCSStzMWJTajlIYlAzTmU5eWRRCk5yVytTYU1hRms2M0puZG81TmUrci9ZYThpemR6RUlyRkw5MWI3Y3RPMWlTZUduWkVXZGxoamtmaEx5NmJSdEYKeUVJclI2TEFra1B2Y3dwVTNWMU5XY3g2QTRuMXgzU3dKdjgxbHdJREFRQUJBb0lCQUJNZXJ4eXR5T3REc0QxbgpZcU1nSTVzK2Z4NmFFckhjRXJqSDJZSE1waWkxVm5SemQrRHhnUnl0M3k3b1BrQlphdVh2eDVVVkMvdDdEZnRlClNBYm9tem5IL3hQckRzUWFFbXBMUFRxMHZlVDluTzZaUlBUU1ZKYjRCWDRXS3NvZ0JyZ2RYOStQa0syWnBBSlQKcWxNcHhsaUNkRXZSdnZRL2xzUTViSEFVL3JlU3FKSFZoKzUycG9YcDZIbnhPNENURWJ5NE15ZzdoYnJoaDFmYgpWL2hMN1o1K3hhR0NqZUxWUmNMWmZENVpKYXh4OStMMlVheFplN3lLZTVlZHhGZGxhYS9vQjJlMkFTR1lnYkRaCjEwY0JsL3g2REJkc1VYckkxcTRmVXNZM3hzZitGTHAwTDNHWUFTZUlLcFpYOXNZdUN5cmx2ZTJqUnZjNm9PR0oKaVY5S2pna0NnWUVBMkJiQ29qV2pWbkFKeUZORnhlSjZxdnM1bUU1SDJBUlFOeWUvdmMvTXpKcm90WDI0ZFlkbQpOZ0hKMVdIcnltbC8yRUt5SkxOcVNqclZHdjJHN2IyM25xTDRUOUliK2FxeUtZMnN0cW9Tbks3TTlsUFN0Z2IzCmRGSDZqajc1b2ZLQjJ3RnIrNitjNENUQXRjMnpxVHcvb3BaSnJRa0ZYem1mekNWU25LckE4azhDZ1lFQTBuUEYKK0VRRHpNWUpTUFRmZW9CTE9GcXRjMHR5MkI4OTRPTHJpckFXdmIvYXZMVG9IVE4xRDB6bHFuZ1ZqQ1JBdTJ3SwpmdkZialVyT1dzU0NPbDZCYWFLMGVFVUw2U0FVT09rQlh4SDh5dnZ2NWRXZk9Jc1U5ZEpaK28yT2RtK2hWNWJkCkZvaHIvejlvd1QwOVgwNFJYUE5TeE9QMXVYUFdoa1QvZG1zY0hqa0NnWUE0UFl4QXJaY3FheFNRdCtPa0FqTU4KQnovUlBTYUR1WE9yTjBRM1FiczYwV0tad2ZQZFd2VW1QMGJwcTRlejhjdGRYTmFDcU5PVUtFWEl0WTJGbU9nTwplTG9LQkZSVm9iQ25FZ0dPdFNzTTdvM1gycTh2d3hacWh0K2dZQkdXcmNoUVdNbGpBeXpnUlpDR2dOZ3V4c2lGCkozcGJkOHFYSXlkTStiWExvc0YvRHdLQmdFbWNPeE9TWHEzaVd0OE0zNW9PZzhEclhwM2tOd0JITlRLU3pJWk8KL3pWUmhPWGFkUkc1ck9rMElXVFY2ZHVCMXE1M3BOZ3YvYkRYQ0lTUkZXZnJKR0xaaVR4RUVsMXhYZ0ZsNXBmbApSOEdNQzZZZGFUcXkweHNFZjNwMnh1ekFNUFBkRGVuU3Y4dWcxemczL2w1MmhQWTVHYXRLZk9sb0RoSWEwaXdPClhPQnBBb0dBVEJBSmFRbHE3R1A0dHBwWVpFV0ozMzlva3gzNnpyYkprbERXcDVYTTBrRXBKb2R3dWFvcE5obzMKb0hiUnZISXhZK3IwWjVabUlNMFVQeC9UblBIMS9FeHNXZ3Z2c0V5TUgyTzN5NUJFaWFVY1dDVGZLRTR5UXNIYQowR09acEhHUzRMZ3JKSHZrZExrVzJtRXVnY25ETEUrT2FBa0VIQTZmOFBWOVVoRzh0bTA9Ci0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg=="

	caCertFileNewBase64 = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURGRENDQWZ5Z0F3SUJBZ0lVY2FDKzlmRnJlSU1rTVlnUHhYQmRVdWRzd1ZVd0RRWUpLb1pJaHZjTkFRRUwKQlFBd0lqRU9NQXdHQTFVRUNoTUZVRk5OUkVJeEVEQU9CZ05WQkFNVEIxSnZiM1FnUTBFd0hoY05NalV4TURFdwpNRFkxT0RBd1doY05NekF4TURBNU1EWTFPREF3V2pBaU1RNHdEQVlEVlFRS0V3VlFVMDFFUWpFUU1BNEdBMVVFCkF4TUhVbTl2ZENCRFFUQ0NBU0l3RFFZSktvWklodmNOQVFFQkJRQURnZ0VQQURDQ0FRb0NnZ0VCQU0rdkEvN1kKdFZlNGtOdzY2RXl6Uzdta3N2WXFpS2Z4ak5DWmhwd2xrM0t2NlFlcXdmdThIWkZIR0VBY05nV09HaWkrellMSgptTlhZdzYzUUMyakIyODE0WnBLdzNZaVhCRnpzMDFuenhIR3ZUWi94b1BiczFacEZtam40M1Y2aDBoK3pPVmdCCnZRd0ZtODgrNjRVdUZocHBhYk5UTStVMFAvMUlVdWJvcC9nSG9ERkdiUVVUNlJvMXhSeSs1M3hWY0JTczkzM0EKTTVTVGVJUnNuVDdkK2dubjRoaTdTdW13MklsaUg3YjN5QUpUNjZXNWxvMTFiajBHb2FiRk5hY0dORm1zN1h2bwo4eldQSElZRXZ5Y2lKbk5mUjNDL1lyQm96Tyt0eWlNN2N6L1IxcGdLVWZtaExETWNtenpTQmdpaE4xcWxrWmZkCm1tL0lRWHJaY3h1R1VPOENBd0VBQWFOQ01FQXdEZ1lEVlIwUEFRSC9CQVFEQWdFR01BOEdBMVVkRXdFQi93UUYKTUFNQkFmOHdIUVlEVlIwT0JCWUVGS3dmN0RnNjlJd0VzbkhmbFp5d0pvZTEwLzYyTUEwR0NTcUdTSWIzRFFFQgpDd1VBQTRJQkFRQ2Z6K08wUnVqUmJKbCt5Wmw1WGhlSmcxcnk3c0pXR2JiV0paOXIzYm4xc01STkhNYXV1NE1SCjdMOFRtd1lJK0M0dndrOWRrQ1MraTdGV2NaL21DWTNXUmo0U3pSVTg5UC9ISllnYklRb1VrVndDUWRyaWpoRXEKUzhieW5YcS9KWGl0aEJYcjhhLzB1cUNPNjZMam81bytOUEJocHVKMTN6dmorR1VrdFEvT29qMkg4OVZrVHNvWQpOVHhacnYzWDhMMzBIeFhFd2ZXd3hXYVBXb2tnNU1DclJabTFJTTA3ZDRYazB4Sy80RkhmMTdEY0lyU2FHaFV6CnBSaWlzazR1aTNzVWNXMkRTRWIzNWRWU2dXWFgzL3JyK2dXVkZSdmdZL1QxejhHTVBjU3hmMjFleTk5VHM1V2UKNS9iZ3U3d1d3MDd6OG1TT1dacXo5SHlCMXlPK2ZuOWsKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	certFileNewBase64   = caCertFileNewBase64
	keyFileNewBase64    = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb3dJQkFBS0NBUUVBejY4RC90aTFWN2lRM0Ryb1RMTkx1YVN5OWlxSXAvR00wSm1HbkNXVGNxL3BCNnJCCis3d2RrVWNZUUJ3MkJZNGFLTDdOZ3NtWTFkakRyZEFMYU1IYnpYaG1rckRkaUpjRVhPelRXZlBFY2E5Tm4vR2cKOXV6Vm1rV2FPZmpkWHFIU0g3TTVXQUc5REFXYnp6N3JoUzRXR21scHMxTXo1VFEvL1VoUzV1aW4rQWVnTVVadApCUlBwR2pYRkhMN25mRlZ3Rkt6M2ZjQXpsSk40aEd5ZFB0MzZDZWZpR0x0SzZiRFlpV0lmdHZmSUFsUHJwYm1XCmpYVnVQUWFocHNVMXB3WTBXYXp0ZStqek5ZOGNoZ1MvSnlJbWMxOUhjTDlpc0dqTTc2M0tJenR6UDlIV21BcFIKK2FFc014eWJQTklHQ0tFM1dxV1JsOTJhYjhoQmV0bHpHNFpRN3dJREFRQUJBb0lCQUY0V0VaOVFtY2JsekdxWgpIVWd5S2VvdVhRejhjL0J4azdPaytjQ2ZuVTdsdHBKTW41amx2aGRrdCszRFdnM21OSitrNFFHUlJ2WUtQNHZzCnBsNk5CSUR2UExqVCsyaTMwYmd2YWdoa1VPaVgzSGpMUkhyWkRHUFppR2NmQVZxdndMdXZ2QmpNb05KamNCVFIKa20xQlZhNGRkMDlRTUVCMERWRTNoS2NyVzMwWFZidnhMMXFrZ2dnQmdXNzBNN2d0UFJzWnlJRU5lUEtTU09mcgprc3V3bHJ2S1ZUQVFFcTdlUWJLaCt0Z0I1ZERneHhzZTcwWVFKd3BmbEc4ZGNhcXc2bmVVaXRXcmRDRDBpTjZJCnZtTnpHNmgyUTdiVFkwb0dSaHRVMk80ZzVxNEpSK1VRS0tXWFQvWmZnVTRQbWxRNTRUdHU0bGVrTmVSa3RURHYKK3A4UmJVRUNnWUVBK2JCRi9mb2RJQ2Zodzc2NGlUM01iSENUNEIxMGh3aWZuWXpVd3N0RDZJc0hESk95Z2FQawpIU3hQVU9Zb2s1UisxUHVQRGxRWDI1clErY29YbnA1ZTBjbjRUOE1pZkJXeHNPSnpxVnh4THVCLzVWS3pvcHRJCnR6MkUvMHRmeTFVaHE2ZWRQY1N2NVlEY1lCTW5IUlEvOFV2NFA3VlBlTjgrSjl0SzdMUHJVM0VDZ1lFQTFPN3UKQmJaeXQ4ei9QQU5hd2pvTEZNZUx1S2tBQ2J6UVdrakN0OGRMNU1zdGRqaWNpS1hsc3BRSkh1c011QUFxeTY5VwoyUmdKUU0xZVBqK2M2RTdFTlZ1NGh2Q0NGMWZOTWM3RXVieXM4SUhOQkJiR3ZFSmN4SzdYRVJLM1dUaHBQWDNlCk5TNWZYd2dRNFNYNEtlSU1BS0tyNC9ZeFBLK0JqTnREL3h3VytsOENnWUVBOS9kc2V6OHlQNlg3MndjRWd4K0IKYmN3YnYzM2hKTjJXanNPMjVFRXpqclRMYWwwZjhRbVBXTDJSZzVrZmdQai9RSXNYVmphRGZ3OXdMREhjWlNXSQpxelcyU1poVUhnRDVkOTVjMlR3NkYwRFRJeTZQd1pRUGtoTWhpdHdUSlg3Rk1wRUNZcjU3cFNQbE4vQ3RibjZXCnhnOFpXa080eWlTQ3VOaGF2MW9yQWJFQ2dZQjMzdUlFT1VldmpTb0tnT0R4QW5nR2hLZDFsejQ4UFIwV0ZtdjMKeGF4RjZ0TjNBRHV1K2FXcnVJYkI3eFRENk9RdXNsQ3ora0lMUnhITS9VYTV5TTNRTkFoWGZzSGRua0lYemcwVgplcy9vdlVuTENYOXJyL2hGaVIvdHJwbWxFb3E4WVVWY3J2UmxyVWJEV1BxeHFWMlVaZjlhWDlnc0Q2bGd3SGN4CkFJRTNCUUtCZ0QrRjlmeGhJRGhWYmc0b0NwMFI5WTExYjNrLy9jbDg5aDdTa2UwVUlSWEhDY1pxVEdiT0ozZzEKUjZIdUVpdEdHdURPSkgvbStRSjQvZTZ6TkM2alk1NElsOXQxVzF6ZXBrZlk3TTZDTTcxenpoanBlamR3SXhDbgo2RnJieURzM0lxT2tBSzE4WFBjMkJob0tram9aK2J2RTQxdk96MlE0V0dSVHQydjBINUYxCi0tLS0tRU5EIFJTQSBQUklWQVRFIEtFWS0tLS0tCg=="
)

func TestSplitHorizonDNSConfigCustomValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		objs         []ctrlclient.Object
		shdcToCreate *enginefeatureseverestv1alpha1.SplitHorizonDNSConfig
		wantErr      error
	}

	testCases := []testCase{
		{
			name: ".spec.baseDomainNameSuffix is missing",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: "",
				},
			},
			wantErr: errInvalidBaseDomainNameSuffix(errors.New("'' is not a valid DNS subdomain: [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")),
		},
		{
			name: ".spec.baseDomainNameSuffix is invalid",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: "invalid_domain",
				},
			},
			wantErr: errInvalidBaseDomainNameSuffix(errors.New("'invalid_domain' is not a valid DNS subdomain: [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")),
		},
		{
			name: ".spec.tls.secretName is missing",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: "",
					},
				},
			},
			wantErr: errTLSSecretNameEmpty,
		},
		{
			name: ".spec.tls.certificate.caCertFile value is missing",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: "",
						},
					},
				},
			},
			wantErr: errTLSCaCertEmpty,
		},
		{
			name: ".spec.tls.certificate.caCertFile value is not base64-encoded",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: "123435",
						},
					},
				},
			},
			wantErr: errTLSCaCertWrongEncoding,
		},
		{
			name: ".spec.tls.certificate.certFile value is missing",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   "",
						},
					},
				},
			},
			wantErr: errTLSCertEmpty,
		},
		{
			name: ".spec.tls.certificate.certFile value is not base64-encoded",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   "123456",
						},
					},
				},
			},
			wantErr: errTLSCertWrongEncoding,
		},
		{
			name: ".spec.tls.certificate.keyFile value is missing",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    "",
						},
					},
				},
			},
			wantErr: errTLSKeyEmpty,
		},
		{
			name: ".spec.tls.certificate.keyFile value is not base64-encoded",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    "123456",
						},
					},
				},
			},
			wantErr: errTLSKeyWrongEncoding,
		},
		{
			name: "secret with .spec.tls.secretName does not exist",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
					},
				},
			},
			wantErr: errors.New("failed to get secrets my-tls-secret: secrets \"my-tls-secret\" not found"),
		},
		{
			name: "all fields are valid, secret with .spec.tls.secretName exists",
			objs: []ctrlclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      certSecretName,
						Namespace: shdcNamespace,
					},
					Data: map[string][]byte{
						"tls.crt": []byte(certFileBase64),
						"tls.key": []byte(keyFileBase64),
						"ca.crt":  []byte(caCertFileBase64),
					},
				},
			},
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "all fields are valid, .spec.tls.certificate is provided",
			shdcToCreate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(enginefeatureseverestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := SplitHorizonDNSConfigCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateCreate(context.TODO(), tc.shdcToCreate)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}

func TestSplitHorizonDNSConfigCustomValidator_ValidateUpdate(t *testing.T) { //nolint:maintidx
	t.Parallel()

	type testCase struct {
		name    string
		objs    []ctrlclient.Object
		oldShdc *enginefeatureseverestv1alpha1.SplitHorizonDNSConfig
		newShdc *enginefeatureseverestv1alpha1.SplitHorizonDNSConfig
		wantErr error
	}

	testCases := []testCase{
		{
			name: ".spec.baseDomainNameSuffix update is not allowed",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: "my-newcompany.com",
				},
			},
			wantErr: errBaseDomainNameSuffixImmutable,
		},
		{
			name: ".spec.tls.secretName update is not allowed",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: "my-tls-secret-new",
					},
				},
			},
			wantErr: errSecretNameImmutable,
		},
		// invalid .spec.tls.certificate values
		{
			name: ".spec.tls.certificate.caCertFile value is missing",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: "",
						},
					},
				},
			},
			wantErr: errTLSCaCertEmpty,
		},
		{
			name: ".spec.tls.certificate.caCertFile value is not base64-encoded",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: "123435",
						},
					},
				},
			},
			wantErr: errTLSCaCertWrongEncoding,
		},
		{
			name: ".spec.tls.certificate.certFile value is missing",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   "",
						},
					},
				},
			},
			wantErr: errTLSCertEmpty,
		},
		{
			name: ".spec.tls.certificate.certFile value is not base64-encoded",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   "123456",
						},
					},
				},
			},
			wantErr: errTLSCertWrongEncoding,
		},
		{
			name: ".spec.tls.certificate.keyFile value is missing",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    "",
						},
					},
				},
			},
			wantErr: errTLSKeyEmpty,
		},
		{
			name: ".spec.tls.certificate.keyFile value is not base64-encoded",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    "123456",
						},
					},
				},
			},
			wantErr: errTLSKeyWrongEncoding,
		},
		// valid update of .spec.tls.certificate
		{
			name: ".spec.tls.certificate update is allowed",
			oldShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			newShdc: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileNewBase64,
							CertFile:   certFileNewBase64,
							KeyFile:    keyFileNewBase64,
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(enginefeatureseverestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := SplitHorizonDNSConfigCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateUpdate(context.TODO(), tc.oldShdc, tc.newShdc)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}

func TestSplitHorizonDNSConfigCustomValidator_ValidateDelete(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		objs         []ctrlclient.Object
		shdcToDelete *enginefeatureseverestv1alpha1.SplitHorizonDNSConfig
		wantErr      error
	}

	testCases := []testCase{
		{
			name: "SplitHorizonDNSConfig is used by some DB Clusters",
			shdcToDelete: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
					Finalizers: []string{
						everestv1alpha1.InUseResourceFinalizer,
					},
				},
				Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
					BaseDomainNameSuffix: shdcBaseDomainSuffix,
					TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
						SecretName: certSecretName,
						Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
							CaCertFile: caCertFileBase64,
							CertFile:   certFileBase64,
							KeyFile:    keyFileBase64,
						},
					},
				},
			},
			wantErr: errDeleteInUse(shdcName),
		},
		{
			name: "SplitHorizonDNSConfig is not used by any DB Cluster",
			objs: []ctrlclient.Object{
				&enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      shdcName,
						Namespace: shdcNamespace,
					},
					Spec: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigSpec{
						BaseDomainNameSuffix: shdcBaseDomainSuffix,
						TLS: enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSSpec{
							SecretName: certSecretName,
							Certificate: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec{
								CaCertFile: caCertFileBase64,
								CertFile:   certFileBase64,
								KeyFile:    keyFileBase64,
							},
						},
					},
				},
			},
			shdcToDelete: &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shdcName,
					Namespace: shdcNamespace,
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(enginefeatureseverestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := SplitHorizonDNSConfigCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateDelete(context.TODO(), tc.shdcToDelete)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}
