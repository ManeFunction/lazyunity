# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- An OpenUPM packages popup: `P` on a project opens it. It lists the packages in four groups: Mane Packages (ManeTools, ManeTools for UI, Sound Maneger), Often used (UniTask, Particle Effect For UGUI, SoftMask For UGUI, UIEffect), Your selection (those of your `config.yml`), and Other installed (the project's other OpenUPM packages, any of which you can remove). A field at the bottom adds any package: lazyunity looks it up in the OpenUPM registry, holding the popup and the app open meanwhile, and lists it in the last group, ticked, to be added when you apply the popup. Installed packages show a green check, and unticking one removes it with `openupm remove`. Ticking a ManeTools module ticks ManeTools, and unticking ManeTools unticks its modules. A package the project has only as a dependency of another one (from `Packages/packages-lock.json`) shows as installed but stays locked while a ticked package needs it. Once nothing needs it, it unlocks, and if you leave it ticked it is added to the manifest at its current version, so you can remove Sound Maneger and keep ManeTools in one pass. `o` opens a package's GitHub page. A package ticked to be added shows a `[↓]`, and closing the popup with changes not applied asks whether to apply or discard them.
- `openupm.packages` in `config.yml` adds your own packages to that popup, in their own group, optionally pinned to a version (`com.example.package@1.2.3`).
- `openupm.showManePackages` and `openupm.showOftenUsed` in `config.yml` show or hide the Mane Packages and Often used groups of that popup. Both are `true` by default.

### Changed

- With the OpenUPM CLI installed, the new project wizard has a sixth step, the OpenUPM packages popup. The ManeTools options left the fifth step.

### Removed

- `ctrl+a` (add an OpenUPM package) in Projects: the field at the bottom of the `P` popup replaces it.

## [0.1.0] - 2026-11-24
First public release of lazyunity: a lazygit-style terminal interface for Unity projects and Editors,
born of an urge to get rid of the slow, bloated, half-a-gig official Unity Hub. Built on top of the official Unity CLI.
Install it with Homebrew from `manefunction/tap`, with Scoop from the `manefunction` bucket,
or with WinGet as `ManeFunction.lazyunity` once Microsoft has reviewed it.
