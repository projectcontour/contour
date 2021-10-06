### New Changelog process

Contour now requires:
- all PRs be labelled with a `release-note/<category>` label, where category
is one of `major`, `minor`, `small`, `docs`, `infra`, or `not-required`.
- a changelog file to be created in `changelogs/unreleased`
for each PR, unless the category is `not-required`. The filename must be
`PR#-githubID-category`, where githubID is that of the
person opening the PR, and the category matches the label category.