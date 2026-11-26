# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- A list of recommended OpenUPM packages: ManeTools, ManeTools for UI, Sound Maneger, UniTask, Particle Effect For UGUI, SoftMask For UGUI and UIEffect. `P` on a project opens it. Installed packages show a green check, and unticking one removes it with `openupm remove`. Ticking a ManeTools module ticks ManeTools, and unticking ManeTools unticks its modules. A package the project has only as a dependency of another one (from `Packages/packages-lock.json`) shows as installed but stays locked while a ticked package needs it. Once nothing needs it, it unlocks, and if you leave it ticked it is added to the manifest at its current version, so you can remove Sound Maneger and keep ManeTools in one pass. `o` opens a package's GitHub page.
- `openupm.packages` in `config.yml` adds your own packages to the end of that list, optionally pinned to a version (`com.example.package@1.2.3`).

### Changed

- With the OpenUPM CLI installed, the new project wizard has a sixth step, the recommended packages. The ManeTools options left the fifth step.

## [0.1.0] - 2026-11-24
First public release of lazyunity: a lazygit-style terminal interface for Unity projects and Editors,
born of an urge to get rid of the slow, bloated, half-a-gig official Unity Hub. Built on top of the official Unity CLI.
Install it with Homebrew from `manefunction/tap`, with Scoop from the `manefunction` bucket,
or with WinGet as `ManeFunction.lazyunity` once Microsoft has reviewed it.
