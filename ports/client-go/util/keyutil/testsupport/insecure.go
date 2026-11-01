// Copyright 2017 The Kubernetes Authors. Licensed under Apache-2.0.
// Weak fixed keys from pinned certificate_manager_test.go; never credentials.
package testsupport
const testGenerateRSAKeyPEMInsecure = `-----BEGIN PRIVATE KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBALB6f6vwyY0IZWcE
Fv8ViUi5kZ64sWUAiWnQhfK0lrAtICJCqy2p95cloaAVNpqvX6uLK3odT0aA7vRZ
PXtYhDtpYTSHvvbAXbHUPl9/u3EPSQ1ByOtdAPio8DmMdWD6xRK4JjfHhH6bLsI+
VHblKik/+BK+xgVo1VuNoRWC8NYRAgMBAAECgYALCs0mgGFSE2CJ5LP+J7YwcFkD
1AVYfyM59VfOPv2vPhGUxzRf/fKtmMePRUiGfvrm2EVDBaa2T/6zmApcZ4ZVeSf8
izNQilInGkCyOQhEbIJ+CE9IUXQG9h5qwdsM9Ehb6+ZkdF/JatkzKtceC0PYlSw8
ZouKqMKMylvEXHLr6QJBAOSY7MRdsip/lAaNJOk/1JCqblV15DS9WpxzDZ1+JBhp
NYrnEy7qe/p1tUKBQMt3nM2Kt1N2HXyPhWVhMvpitkkCQQDFoi2T9T31nVm/4HFV
ALK5wt3FU0MLLn+E+km/++SUjVebtjCC5Gxz+ByAsj963jhCdh+2EzgyPkOKGK5a
LwGJAkBBjuXgDuroqzvdgR8D0a15a5dG5Q90XJWe5pQSBbn+UjXrxwdGXjL+CkHY
d88ISx5qCA05X1dngJWGFJEVI7gZAkAbWHFOA6TrEzaT4g5MYKhaI6hj4T1pkql6
UNdbhRL/qv7wQKk9szV+ZlorRH6cFZtbNtT0cHxaF1tpBDk7qT1hAkEAs8WLPccN
N2BAYjQjJanWfMAIyHPdwenmKhlvkWFvnwZWQecHXDmfHZlEREqpkd9IxOIGNii7
OiCuooK0UDdbPw==
-----END PRIVATE KEY-----
`
const testGenerateECKeyPEMInsecure = `-----BEGIN PRIVATE KEY-----
MHgCAQAwEAYHKoZIzj0CAQYFK4EEACEEYTBfAgEBBByl2nOZXdac4dWx0UvrHn8G
P3p+oI+mjH6Znno3oTwDOgAE5/S2xcvEyJWSOjNilulwy9YnPOPubuZ4ztOh95kU
eaVq8eqxqzGSxwanMrwbD/1PQ97kNdi53No=
-----END PRIVATE KEY-----
`
func WeakFixture(name any)any{if name.(string)=="rsa"{return testGenerateRSAKeyPEMInsecure};return testGenerateECKeyPEMInsecure}
