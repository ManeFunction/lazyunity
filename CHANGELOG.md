# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- An OpenUPM packages popup: press `P` on a project. It lists packages in four groups:
  - **Mane Packages:** ManeTools, ManeTools for UI and Sound Maneger.
  - **Often used:** UniTask, Particle Effect For UGUI, SoftMask For UGUI and UIEffect.
  - **Your selection:** the packages from your `config.yml`.
  - **Other installed:** the project's other OpenUPM packages, any of which you can remove.

  Installed packages show a green check; untick one to remove it with `openupm remove`. A package ticked to be added shows `[↓]`. `o` opens a package's GitHub page. If you close the popup with changes not applied, it asks whether to apply or discard them.
- A field at the bottom of that popup adds any package. lazyunity looks it up in the OpenUPM registry and adds it to the last group, ticked; the package installs when you apply the popup. During the lookup, the popup and the app don't close.
- Dependencies in that popup. Ticking a ManeTools module ticks ManeTools, and unticking ManeTools unticks its modules. A package the project has only as a dependency of another one (from `Packages/packages-lock.json`) shows as installed and stays locked while a ticked package needs it. Once nothing needs it, it unlocks; if you leave it ticked, lazyunity adds it to the manifest at its current version. So you can remove Sound Maneger and keep ManeTools in one pass.
- `openupm.packages` in `config.yml` adds your own packages to that popup, in their own group, optionally pinned to a version (`com.example.package@1.2.3`).
- `openupm.showManePackages` and `openupm.showOftenUsed` in `config.yml` show or hide the Mane Packages and Often used groups. Both are `true` by default.

### Changed

- With the OpenUPM CLI installed, the new project wizard has a sixth step: the OpenUPM packages popup. The ManeTools options moved there from the fifth step.

### Removed

- `ctrl+a` (add an OpenUPM package) in Projects: use the field at the bottom of the `P` popup instead.

## [0.1.0] - 2026-11-24
First public release of lazyunity: a lazygit-style terminal interface for Unity projects and Editors,
born of an urge to get rid of the slow, bloated, half-a-gig official Unity Hub. Built on top of the official Unity CLI.
Install it with Homebrew from `manefunction/tap`, with Scoop from the `manefunction` bucket,
or with WinGet as `ManeFunction.lazyunity` once Microsoft has reviewed it.
