# Git Workflow

All development happens in the `main` branch, and all PRs should be submitted to the `main` branch.

Releases follow [SemVer](https://semver.org/) and start from the `main` branch as `release/vX.Y` branches (with the prefix `v` to distinguish between versions and version system tracking). Releases contain only merged or cherry-picked commits from the `main` branch. 

Release branches are tagged with `vX.Y.Z` tags, and they also could be tagged with `vX.Y.X-rcN` for testing purposes.

```mermaid
%%{init: { 'logLevel': 'debug', 'theme': 'base' } }%%
gitGraph
       commit
       commit
       commit
       branch release/v1.0
       checkout release/v1.0
       commit tag: "v1.0.0"
       checkout main
       commit
       commit id: "fix_1"
       checkout release/v1.0
       cherry-pick id: "fix_1"
       commit tag: "v1.0.1"
       checkout main
       commit
       commit
       commit
       branch release/v1.1
       checkout release/v1.1
       commit tag: "v1.1.0"
       checkout main
       commit
       commit
       commit
       commit
       commit
       commit
       branch release/v2.0
       checkout release/v2.0
       commit tag: "v2.0.0"
       checkout main
       commit
       commit
```

## Images

Commits pushed to the `release/vX.Y` branch and tags with `rc` in their names would initiate build and push to the `docker.io/peconalab` registry.

Pushing new tag `vX.Y.X` would initiate build and push to the `docker.io/pecona` registry.

Please check also [Build and Push images](../../.github/workflows/build-push-images.yaml).
