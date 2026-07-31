# Changelog

## [0.5.0](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.4.1...v0.5.0) (2026-07-31)


### Features

* integrate Zarf output with Terraform logs ([#293](https://github.com/defenseunicorns/terraform-provider-uds/issues/293)) ([1e36c78](https://github.com/defenseunicorns/terraform-provider-uds/commit/1e36c78ba675471c2d79155cc714a5b7fd1951a9))


### Bug Fixes

* exclude sboms from release checksums ([#301](https://github.com/defenseunicorns/terraform-provider-uds/issues/301)) ([94875f9](https://github.com/defenseunicorns/terraform-provider-uds/commit/94875f9bdd9e8b3b80c44bc0dae32c874f43fcf1))


### Miscellaneous

* add security.md ([#298](https://github.com/defenseunicorns/terraform-provider-uds/issues/298)) ([fe82e8f](https://github.com/defenseunicorns/terraform-provider-uds/commit/fe82e8ff0b0d43c004c9e77cabf43596065707f8))
* clean up tf docs and add generate docs workflow/lint ([#303](https://github.com/defenseunicorns/terraform-provider-uds/issues/303)) ([3b6d648](https://github.com/defenseunicorns/terraform-provider-uds/commit/3b6d648276199e44459224a05cf80bc6d5ce41e6))
* **deps:** update support dependencies to v1.50.0 ([#302](https://github.com/defenseunicorns/terraform-provider-uds/issues/302)) ([e117bc8](https://github.com/defenseunicorns/terraform-provider-uds/commit/e117bc8eab731dea96905f55e010c4688b6c0a54))
* **deps:** update support-deps ([#296](https://github.com/defenseunicorns/terraform-provider-uds/issues/296)) ([dceca44](https://github.com/defenseunicorns/terraform-provider-uds/commit/dceca4455272b0c086b10e6edfcbdd548f5974d1))
* improve developer ergonomics and tasks ([#294](https://github.com/defenseunicorns/terraform-provider-uds/issues/294)) ([f865465](https://github.com/defenseunicorns/terraform-provider-uds/commit/f865465ec353e47b502060f9f14974c79fbdf11c))
* update nightly release ([#304](https://github.com/defenseunicorns/terraform-provider-uds/issues/304)) ([2742f84](https://github.com/defenseunicorns/terraform-provider-uds/commit/2742f848e8215af867639ce71286e4220959b790))
* update release binary arch/os to recommended ([#299](https://github.com/defenseunicorns/terraform-provider-uds/issues/299)) ([bfe430d](https://github.com/defenseunicorns/terraform-provider-uds/commit/bfe430d24fa04dd46b2c19d9bbe79153a87222ea))
* update renovate to group mise deps ([#300](https://github.com/defenseunicorns/terraform-provider-uds/issues/300)) ([60e0cc1](https://github.com/defenseunicorns/terraform-provider-uds/commit/60e0cc1e8ebdcdcaa81682ba0e1946cfd7aed7bb))

## [0.4.1](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.4.0...v0.4.1) (2026-07-28)


### Bug Fixes

* breaking changes in zarf v0.81.0 and oras-go v2.6.1 ([#277](https://github.com/defenseunicorns/terraform-provider-uds/issues/277)) ([a96941d](https://github.com/defenseunicorns/terraform-provider-uds/commit/a96941de9d5742e83d53e9202941a44f5ee40bb7))
* **deps:** update application-deps ([#288](https://github.com/defenseunicorns/terraform-provider-uds/issues/288)) ([3d3cf09](https://github.com/defenseunicorns/terraform-provider-uds/commit/3d3cf097483d127b27558365571e313823c1fafe))
* **deps:** update zarf ([#281](https://github.com/defenseunicorns/terraform-provider-uds/issues/281)) ([e27400a](https://github.com/defenseunicorns/terraform-provider-uds/commit/e27400a2586bfcefc76f1513f59d4ed1824780c6))
* **deps:** update zarf to v0.80.0 ([#257](https://github.com/defenseunicorns/terraform-provider-uds/issues/257)) ([a6cb149](https://github.com/defenseunicorns/terraform-provider-uds/commit/a6cb14945b2f02feb30e1cc05140991d9d052d28))
* **deps:** update zarf to v0.81.0 ([#268](https://github.com/defenseunicorns/terraform-provider-uds/issues/268)) ([f690048](https://github.com/defenseunicorns/terraform-provider-uds/commit/f6900488594a7d353c68d1f45211f9c7fdb2245f))
* **deps:** update zarf to v0.81.1 ([#274](https://github.com/defenseunicorns/terraform-provider-uds/issues/274)) ([94abb7d](https://github.com/defenseunicorns/terraform-provider-uds/commit/94abb7d109c00680426db720d5930ee427693679))
* leak in yaml parser ([#276](https://github.com/defenseunicorns/terraform-provider-uds/issues/276)) ([ca6863f](https://github.com/defenseunicorns/terraform-provider-uds/commit/ca6863f638304d8e93b4f529e041c0a21406f1f5))


### Miscellaneous

* add gpg signing to release workflow ([#289](https://github.com/defenseunicorns/terraform-provider-uds/issues/289)) ([f489622](https://github.com/defenseunicorns/terraform-provider-uds/commit/f489622aac51683b66e3d06e7b6c22b7f95d4d51))
* add zarf init to renovate zarf grouping ([#270](https://github.com/defenseunicorns/terraform-provider-uds/issues/270)) ([f55e6a1](https://github.com/defenseunicorns/terraform-provider-uds/commit/f55e6a1f7ca05f3192bc634bb3d8bad0fffa9e2d))
* cleanup readme; add templates; add shim workflow for test ([#290](https://github.com/defenseunicorns/terraform-provider-uds/issues/290)) ([a845910](https://github.com/defenseunicorns/terraform-provider-uds/commit/a8459106dd4a16f9e8516d2547498e61c0a6c980))
* cleanup renovate config/group things better ([#284](https://github.com/defenseunicorns/terraform-provider-uds/issues/284)) ([61861d5](https://github.com/defenseunicorns/terraform-provider-uds/commit/61861d54cc9334ca95daafd6093e005c34ac9e65))
* **deps:** update dependency defenseunicorns/uds-cli to v0.34.0 ([#263](https://github.com/defenseunicorns/terraform-provider-uds/issues/263)) ([3a9e1c2](https://github.com/defenseunicorns/terraform-provider-uds/commit/3a9e1c2a1514c832e83150077dc410f9d41b3a55))
* **deps:** update dependency defenseunicorns/uds-cli to v0.34.1 ([#271](https://github.com/defenseunicorns/terraform-provider-uds/issues/271)) ([87beb1f](https://github.com/defenseunicorns/terraform-provider-uds/commit/87beb1fabee6a78dc559c20af57dc8989e62ec56))
* **deps:** update dependency defenseunicorns/uds-cli to v0.34.2 ([#275](https://github.com/defenseunicorns/terraform-provider-uds/issues/275)) ([bb85066](https://github.com/defenseunicorns/terraform-provider-uds/commit/bb850663af3925c1897e57a4c4a141ea67597435))
* **deps:** update dependency defenseunicorns/uds-cli to v0.34.3 ([#282](https://github.com/defenseunicorns/terraform-provider-uds/issues/282)) ([5c765a3](https://github.com/defenseunicorns/terraform-provider-uds/commit/5c765a39a538778309a47bbf83f8fe9ce3a13582))
* **deps:** update dependency defenseunicorns/uds-k3d to v0.20.2 ([#265](https://github.com/defenseunicorns/terraform-provider-uds/issues/265)) ([843a25f](https://github.com/defenseunicorns/terraform-provider-uds/commit/843a25f44377d463e2f99f1ee462fbafa192cdff))
* **deps:** update ghcr.io/stefanprodan/charts/podinfo docker tag to v6.14.0 ([#248](https://github.com/defenseunicorns/terraform-provider-uds/issues/248)) ([ae75dec](https://github.com/defenseunicorns/terraform-provider-uds/commit/ae75dec440904a6ae239759811d6b661a6b94ce4))
* **deps:** update ghcr.io/stefanprodan/podinfo docker tag to v6.14.0 ([#249](https://github.com/defenseunicorns/terraform-provider-uds/issues/249)) ([083f20f](https://github.com/defenseunicorns/terraform-provider-uds/commit/083f20fbeba41d823b2dd9436f5b9aefb5b08fe0))
* **deps:** update ghcr.io/zarf-dev/packages/init docker tag to v0.80.0 ([#258](https://github.com/defenseunicorns/terraform-provider-uds/issues/258)) ([4a175af](https://github.com/defenseunicorns/terraform-provider-uds/commit/4a175afa2790b821914ee574a846b3e05a1c80ec))
* **deps:** update ghcr.io/zarf-dev/packages/init docker tag to v0.81.0 ([#267](https://github.com/defenseunicorns/terraform-provider-uds/issues/267)) ([3ab4c62](https://github.com/defenseunicorns/terraform-provider-uds/commit/3ab4c625ffa596628c0bb845f69e6ecc44d63e71))
* **deps:** update github actions to v7 ([#254](https://github.com/defenseunicorns/terraform-provider-uds/issues/254)) ([67c70c9](https://github.com/defenseunicorns/terraform-provider-uds/commit/67c70c95099fa45c8ef2b0b777c15952bd78b06d))
* **deps:** update github-actions ([#255](https://github.com/defenseunicorns/terraform-provider-uds/issues/255)) ([19fccac](https://github.com/defenseunicorns/terraform-provider-uds/commit/19fccac783d2509cb294195ef2a87a36c4c089bc))
* **deps:** update github-actions ([#264](https://github.com/defenseunicorns/terraform-provider-uds/issues/264)) ([4e7a4c7](https://github.com/defenseunicorns/terraform-provider-uds/commit/4e7a4c7f76e225aa2531ab82f1d0c350db13a3a6))
* **deps:** update github-actions ([#266](https://github.com/defenseunicorns/terraform-provider-uds/issues/266)) ([7cdd3fd](https://github.com/defenseunicorns/terraform-provider-uds/commit/7cdd3fd9cbb7d9e84561ba276b208c6ea7336a41))
* **deps:** update github-actions ([#269](https://github.com/defenseunicorns/terraform-provider-uds/issues/269)) ([6d29c65](https://github.com/defenseunicorns/terraform-provider-uds/commit/6d29c651c2ee8c3f39e0872971b7b0ff836cfa02))
* **deps:** update github-actions ([#272](https://github.com/defenseunicorns/terraform-provider-uds/issues/272)) ([24da1a1](https://github.com/defenseunicorns/terraform-provider-uds/commit/24da1a167fb8e25c6f26357fd6b48a291cb15d48))
* **deps:** update github-actions ([#280](https://github.com/defenseunicorns/terraform-provider-uds/issues/280)) ([5c010d1](https://github.com/defenseunicorns/terraform-provider-uds/commit/5c010d1047d529d498d63e84adfec4ee3f5058b4))
* **deps:** update github-actions to v7 ([#273](https://github.com/defenseunicorns/terraform-provider-uds/issues/273)) ([8eb5789](https://github.com/defenseunicorns/terraform-provider-uds/commit/8eb5789f85176b725171c46b9d5c5c91d7eac12e))
* **deps:** update support-deps ([#286](https://github.com/defenseunicorns/terraform-provider-uds/issues/286)) ([109d39f](https://github.com/defenseunicorns/terraform-provider-uds/commit/109d39f39cf36f2fe731e8af806efd2346c9012c))
* **deps:** update zarf to v1.3.0 ([#291](https://github.com/defenseunicorns/terraform-provider-uds/issues/291)) ([e4918ec](https://github.com/defenseunicorns/terraform-provider-uds/commit/e4918ec99a5e8fc4c725a20ad1dbddfd98979be6))
* github workflow cleanliness ([#283](https://github.com/defenseunicorns/terraform-provider-uds/issues/283)) ([76588ac](https://github.com/defenseunicorns/terraform-provider-uds/commit/76588ac2366853f6b9264ae0d9f44dc8700ea97f))
* more renovate/release please cleanup/fixes ([#287](https://github.com/defenseunicorns/terraform-provider-uds/issues/287)) ([006acfe](https://github.com/defenseunicorns/terraform-provider-uds/commit/006acfe03704385ad74c23c09eb875e788d8e6b0))
* replace archived go-yamlv2 with goccy/go-yaml ([#292](https://github.com/defenseunicorns/terraform-provider-uds/issues/292)) ([f011aab](https://github.com/defenseunicorns/terraform-provider-uds/commit/f011aab78621f6ff8b81d5521a122781e0c542a2))

## [0.4.0](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.5...v0.4.0) (2026-06-18)


### ⚠ BREAKING CHANGES

* **uds_package:** replace scalar `timeout` with per-operation wall-clock `timeouts` meta attr ([#244](https://github.com/defenseunicorns/terraform-provider-uds/issues/244))
* remove unused bundle_metadata resource ([#237](https://github.com/defenseunicorns/terraform-provider-uds/issues/237))
* keyless package signature verification with zarf dep v0.77.0 update (https://github.com/defenseunicorns/terraform-provider-uds/pull/236)

### Features

* add support for zarf values ([#233](https://github.com/defenseunicorns/terraform-provider-uds/issues/233)) ([0a76103](https://github.com/defenseunicorns/terraform-provider-uds/commit/0a76103eee9ca855fa4d717c5b51e22d8a70c7c0))
* keyless package signature verification with zarf dep v0.77.0 update (https://github.com/defenseunicorns/terraform-provider-uds/pull/236) ([61866ad](https://github.com/defenseunicorns/terraform-provider-uds/commit/61866add0a6a41cf237dc1c61a4ef21fd9317c8d))
* **uds_package:** add alpha `optional_components` for optional component selection ([#238](https://github.com/defenseunicorns/terraform-provider-uds/issues/238)) ([d995f5b](https://github.com/defenseunicorns/terraform-provider-uds/commit/d995f5bc85f46b5dd5856a4a0f8bdb145a26b299))


### Bug Fixes

* **deps:** update go-dependencies to v2.6.1 ([#240](https://github.com/defenseunicorns/terraform-provider-uds/issues/240)) ([efeece4](https://github.com/defenseunicorns/terraform-provider-uds/commit/efeece44f9f9b1830bd3990a9d84e0b95e9ac36e))
* **deps:** update go-dependencies to v3 ([#232](https://github.com/defenseunicorns/terraform-provider-uds/issues/232)) ([5f5f21b](https://github.com/defenseunicorns/terraform-provider-uds/commit/5f5f21b07ba2d4395aee0f2c8dbb912150c547cb))


### Refactoring

* **uds_package:** replace scalar `timeout` with per-operation wall-clock `timeouts` meta attr ([#244](https://github.com/defenseunicorns/terraform-provider-uds/issues/244)) ([6490592](https://github.com/defenseunicorns/terraform-provider-uds/commit/64905922e01145f9a38c3abc530ee02f622eb657))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.32.0 ([#234](https://github.com/defenseunicorns/terraform-provider-uds/issues/234)) ([244356d](https://github.com/defenseunicorns/terraform-provider-uds/commit/244356d3c45a00baf1f5adcbee2f57cca2838172))
* **deps:** update dependency defenseunicorns/uds-cli to v0.33.0 ([#246](https://github.com/defenseunicorns/terraform-provider-uds/issues/246)) ([c27e41c](https://github.com/defenseunicorns/terraform-provider-uds/commit/c27e41c1a6776dc7da2d6f7e8c1e077431be62d2))
* **deps:** update dependency defenseunicorns/uds-k3d to v0.20.1 ([#242](https://github.com/defenseunicorns/terraform-provider-uds/issues/242)) ([4c30157](https://github.com/defenseunicorns/terraform-provider-uds/commit/4c30157bb858a334af30709e2b21e009c7f69df8))
* **deps:** update github-actions ([#224](https://github.com/defenseunicorns/terraform-provider-uds/issues/224)) ([7f2c287](https://github.com/defenseunicorns/terraform-provider-uds/commit/7f2c287eb855641628c3216b96fe274736db698d))
* **deps:** update github-actions ([#231](https://github.com/defenseunicorns/terraform-provider-uds/issues/231)) ([d630b77](https://github.com/defenseunicorns/terraform-provider-uds/commit/d630b777042bb8ce40e9d830efcb8e7f05aa8bf7))
* **deps:** update github-actions ([#239](https://github.com/defenseunicorns/terraform-provider-uds/issues/239)) ([a6285f8](https://github.com/defenseunicorns/terraform-provider-uds/commit/a6285f88fe0fcf01aab660e57fc64b80e1cc3a2a))
* **deps:** update github-actions ([#247](https://github.com/defenseunicorns/terraform-provider-uds/issues/247)) ([4fe2a29](https://github.com/defenseunicorns/terraform-provider-uds/commit/4fe2a292216485fb0cc8da290941e2998fe8c7fd))
* **deps:** update github-actions to v0.32.0 ([#235](https://github.com/defenseunicorns/terraform-provider-uds/issues/235)) ([77d600b](https://github.com/defenseunicorns/terraform-provider-uds/commit/77d600b6e168f307b43eef11350fb2bfdc5653bd))
* remove unused bundle_metadata resource ([#237](https://github.com/defenseunicorns/terraform-provider-uds/issues/237)) ([6e89c9e](https://github.com/defenseunicorns/terraform-provider-uds/commit/6e89c9e6e262d5d3ed33c0939418681b3aefc29c))
* rename LICENSE.md to LICENSE for standard tooling recognition ([#225](https://github.com/defenseunicorns/terraform-provider-uds/issues/225)) ([0530c3f](https://github.com/defenseunicorns/terraform-provider-uds/commit/0530c3f43ac209281df373c05342c0f8175db951))
* upgrade zarf to v0.79.0 / add acc test and example using oci ref ([#250](https://github.com/defenseunicorns/terraform-provider-uds/issues/250)) ([68046d8](https://github.com/defenseunicorns/terraform-provider-uds/commit/68046d83fe97291df6ce6ebbf2b0f8d935b30ca6))

## [0.3.5](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.4...v0.3.5) (2026-05-15)


### Bug Fixes

* **deps:** update zarf to v0.76.0 ([#222](https://github.com/defenseunicorns/terraform-provider-uds/issues/222)) ([2101dd7](https://github.com/defenseunicorns/terraform-provider-uds/commit/2101dd7c80c25fe92e5063ce241f01ce59f6bc1c))


### Miscellaneous

* **config:** migrate config renovate.json ([#218](https://github.com/defenseunicorns/terraform-provider-uds/issues/218)) ([4c7775b](https://github.com/defenseunicorns/terraform-provider-uds/commit/4c7775ba54ebf7430510bce5b2b710a59cc9a81d))
* **deps:** update dependency defenseunicorns/uds-cli to v0.31.0 ([#223](https://github.com/defenseunicorns/terraform-provider-uds/issues/223)) ([54b5a57](https://github.com/defenseunicorns/terraform-provider-uds/commit/54b5a570bdf82f8b69f7c760dafeebd1ea86bf3d))
* **deps:** update github actions to v5 ([#211](https://github.com/defenseunicorns/terraform-provider-uds/issues/211)) ([f8bd29b](https://github.com/defenseunicorns/terraform-provider-uds/commit/f8bd29b6c7098db88465bcee4334febce0c630b4))
* **deps:** update github-actions ([#207](https://github.com/defenseunicorns/terraform-provider-uds/issues/207)) ([b8264ba](https://github.com/defenseunicorns/terraform-provider-uds/commit/b8264ba9ad508542d391d03d87f5083226df22e3))
* **deps:** update github-actions ([#221](https://github.com/defenseunicorns/terraform-provider-uds/issues/221)) ([293fe79](https://github.com/defenseunicorns/terraform-provider-uds/commit/293fe7923cacab7710afeaf4fbe1dd8b50f1fa4b))
* **deps:** update github-actions to v21 ([#219](https://github.com/defenseunicorns/terraform-provider-uds/issues/219)) ([f879a5a](https://github.com/defenseunicorns/terraform-provider-uds/commit/f879a5a417a032883e558b296409345ae6f0deec))

## [0.3.4](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.3...v0.3.4) (2026-05-01)


### Bug Fixes

* **deps:** update go-dependencies ([#213](https://github.com/defenseunicorns/terraform-provider-uds/issues/213)) ([a7e952e](https://github.com/defenseunicorns/terraform-provider-uds/commit/a7e952e459ef631bc555d9046a1b75dfb6ed8c1d))
* **deps:** update zarf to v0.75.1 ([#215](https://github.com/defenseunicorns/terraform-provider-uds/issues/215)) ([3404357](https://github.com/defenseunicorns/terraform-provider-uds/commit/340435735cfadb99137dfa5c640b4ba3a8e57752))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.30.4 ([#216](https://github.com/defenseunicorns/terraform-provider-uds/issues/216)) ([c317a5c](https://github.com/defenseunicorns/terraform-provider-uds/commit/c317a5c9aef805111089a4e2651ece878a0acac1))
* **deps:** update dependency defenseunicorns/uds-k3d to v0.20.0 ([#210](https://github.com/defenseunicorns/terraform-provider-uds/issues/210)) ([2acb8a8](https://github.com/defenseunicorns/terraform-provider-uds/commit/2acb8a8a2ec6502ef48341680d20211c70a55784))
* **deps:** update go dependencies to v1.3.2 ([#208](https://github.com/defenseunicorns/terraform-provider-uds/issues/208)) ([275f914](https://github.com/defenseunicorns/terraform-provider-uds/commit/275f914312a1629b214236eba26b474abd7dae67))

## [0.3.3](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.2...v0.3.3) (2026-04-17)


### Bug Fixes

* **deps:** update go dependencies to v4.1.4 ([#202](https://github.com/defenseunicorns/terraform-provider-uds/issues/202)) ([04caa41](https://github.com/defenseunicorns/terraform-provider-uds/commit/04caa4109e46fc66eeac26db4e2ccd7a872c5630))
* **deps:** update zarf to v0.75.0 ([#204](https://github.com/defenseunicorns/terraform-provider-uds/issues/204)) ([79d86ed](https://github.com/defenseunicorns/terraform-provider-uds/commit/79d86ed545007d3729441190af65a2627bde2c82))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.30.3 ([#205](https://github.com/defenseunicorns/terraform-provider-uds/issues/205)) ([51e714a](https://github.com/defenseunicorns/terraform-provider-uds/commit/51e714adb9469122335b6b27f845215e2d4a04fd))
* **deps:** update github actions to v0.30.3 ([#206](https://github.com/defenseunicorns/terraform-provider-uds/issues/206)) ([07bb566](https://github.com/defenseunicorns/terraform-provider-uds/commit/07bb5668a77865b4b7203d6dc89f0c52551e7079))
* **deps:** update github-actions ([#201](https://github.com/defenseunicorns/terraform-provider-uds/issues/201)) ([542406c](https://github.com/defenseunicorns/terraform-provider-uds/commit/542406ca813565f8c92939d33a5ee390cde79843))

## [0.3.2](https://github.com/defenseunicorns/terraform-provider-uds/compare/v0.3.1...v0.3.2) (2026-04-09)


### Bug Fixes

* **deps:** update zarf to v0.74.2 ([#198](https://github.com/defenseunicorns/terraform-provider-uds/issues/198)) ([dc43720](https://github.com/defenseunicorns/terraform-provider-uds/commit/dc43720a1e0425b46b395168f25db71bc840df16))


### Miscellaneous

* **deps:** update dependency defenseunicorns/uds-cli to v0.30.2 ([#200](https://github.com/defenseunicorns/terraform-provider-uds/issues/200)) ([f374d6d](https://github.com/defenseunicorns/terraform-provider-uds/commit/f374d6d195b8715771cc8c3f8ec2d13db38ef2db))
* **deps:** update github actions to v0.30.2 ([#197](https://github.com/defenseunicorns/terraform-provider-uds/issues/197)) ([cd78b21](https://github.com/defenseunicorns/terraform-provider-uds/commit/cd78b21c1b1b89026a0d23268ac9870dde4297e7))

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
