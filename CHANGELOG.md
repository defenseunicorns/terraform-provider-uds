# Changelog

## [0.3.1](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.0...v0.3.1) (2026-04-03)


### Bug Fixes

* **deps:** update go-dependencies ([#183](https://github.com/defenseunicorns/terraform-provider-uds/issues/183)) ([358d744](https://github.com/defenseunicorns/terraform-provider-uds/commit/358d74413afff97e6b24f0b20029ced8959bdc21))
* **deps:** update zarf to v0.74.1 ([#194](https://github.com/defenseunicorns/terraform-provider-uds/issues/194)) ([0144b8b](https://github.com/defenseunicorns/terraform-provider-uds/commit/0144b8b14016eff391df452c7040a5230c4b1042))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.30.1 ([#195](https://github.com/defenseunicorns/terraform-provider-uds/issues/195)) ([2fe5a03](https://github.com/defenseunicorns/terraform-provider-uds/commit/2fe5a0374565fc2bbca4323dd5f6acef4e1fe6d3))
* **deps:** update github-actions ([#196](https://github.com/defenseunicorns/terraform-provider-uds/issues/196)) ([63aa247](https://github.com/defenseunicorns/terraform-provider-uds/commit/63aa247d7ca3df707165aeb1a24e00f020d2f1d3))

## [0.3.0](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.2.2...v0.3.0) (2026-03-20)


### Features

* force helm ssa conflicts provider flag with zarf v0.74.0 ([#191](https://github.com/defenseunicorns/terraform-provider-uds/issues/191)) ([98b58cd](https://github.com/defenseunicorns/terraform-provider-uds/commit/98b58cd9ec632ffea9f92bd6f1da6f9cebe70953))
* return all variables set during package deploy in `set_variables` attribute for uds_package ([#190](https://github.com/defenseunicorns/terraform-provider-uds/issues/190)) ([8c94a8d](https://github.com/defenseunicorns/terraform-provider-uds/commit/8c94a8d584acc0fd1982b3475468401a6df10795))
* return variables set in package deploy actions for `uds_package` resource ([#175](https://github.com/defenseunicorns/terraform-provider-uds/issues/175)) ([d973bfc](https://github.com/defenseunicorns/terraform-provider-uds/commit/d973bfca43f6e4ae7e6a380938424f8c744afc04))


### Miscellaneous

* **deps:** update github-actions ([#181](https://github.com/defenseunicorns/terraform-provider-uds/issues/181)) ([44b06ac](https://github.com/defenseunicorns/terraform-provider-uds/commit/44b06ac0924893d26202f3b07db173424ab48b19))
* remove invalid/flaky unit test for different-cased map keys in set variables returned from deploy ([#192](https://github.com/defenseunicorns/terraform-provider-uds/issues/192)) ([49873a6](https://github.com/defenseunicorns/terraform-provider-uds/commit/49873a68d68c09d68d0400db6a7411942b83ff4d))

## [0.2.2](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.2.1...v0.2.2) (2026-03-05)


### Bug Fixes

* regression that blocks non-k8s packages from deploying ([#179](https://github.com/defenseunicorns/terraform-provider-uds/issues/179)) ([b3bbeaa](https://github.com/defenseunicorns/terraform-provider-uds/commit/b3bbeaad69f2a84772132549e1331dc44311cfd0))


### Miscellaneous

* **deps:** update github actions to 88cf7cc ([#178](https://github.com/defenseunicorns/terraform-provider-uds/issues/178)) ([2913eb7](https://github.com/defenseunicorns/terraform-provider-uds/commit/2913eb7f0a54a549c3c78b45d7b4e2a28c04a5a9))

## [0.2.1](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.2.0...v0.2.1) (2026-03-04)


### Bug Fixes

* **deps:** update go-dependencies ([#172](https://github.com/defenseunicorns/terraform-provider-uds/issues/172)) ([656b512](https://github.com/defenseunicorns/terraform-provider-uds/commit/656b51286a7076a9b317ecc4c1fc542e37eda6d6))
* **deps:** update zarf to v0.73.1 ([#176](https://github.com/defenseunicorns/terraform-provider-uds/issues/176)) ([5a0f177](https://github.com/defenseunicorns/terraform-provider-uds/commit/5a0f177deb52ce360deaeb740bb8e498104d61d5))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.28.4 ([#170](https://github.com/defenseunicorns/terraform-provider-uds/issues/170)) ([ac03a96](https://github.com/defenseunicorns/terraform-provider-uds/commit/ac03a96d92d89e011ec4f7f224d3ce1720280429))
* **deps:** update github-actions ([#168](https://github.com/defenseunicorns/terraform-provider-uds/issues/168)) ([37cc4fd](https://github.com/defenseunicorns/terraform-provider-uds/commit/37cc4fd662073998d826479a4c5682f15e7bad38))
* **deps:** update github-actions to v7 ([#165](https://github.com/defenseunicorns/terraform-provider-uds/issues/165)) ([29988d1](https://github.com/defenseunicorns/terraform-provider-uds/commit/29988d19e7838f9e338bb9a181a71cc49d9af939))

## [0.2.0](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.1.6...v0.2.0) (2026-02-23)


### Features

* import uds_package by ID ([#155](https://github.com/defenseunicorns/terraform-provider-uds/issues/155)) ([f959433](https://github.com/defenseunicorns/terraform-provider-uds/commit/f959433706486f6b8b76e0efe9cf6037984996c7))


### Bug Fixes

* **deps:** update zarf to v0.72.0 ([#157](https://github.com/defenseunicorns/terraform-provider-uds/issues/157)) ([0ec10a1](https://github.com/defenseunicorns/terraform-provider-uds/commit/0ec10a198a179c565faab371dd072587585dc165))
* **deps:** update zarf to v0.73.0 ([#164](https://github.com/defenseunicorns/terraform-provider-uds/issues/164)) ([d52e330](https://github.com/defenseunicorns/terraform-provider-uds/commit/d52e330bd8c741e7faf347e5def79df237a83be6))
* enable destruction of package resources using namespace overrides ([#167](https://github.com/defenseunicorns/terraform-provider-uds/issues/167)) ([c8c8add](https://github.com/defenseunicorns/terraform-provider-uds/commit/c8c8add84e0eaf55985f272e960a75393841b88c))
