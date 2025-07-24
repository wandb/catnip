# Releasing

We use goreleaser in github actions to build our CLI. We also push the latest docker image to dockerhub whenever we see a push to a tag of vX.X.X. For the CLI all assets are stored in github releases as well as Cloudflare R2. See the [release](../.github/workflows/release.yml) and [build](../.github/workflows/build-container.yml) workflows.

We have a CLI [installation script](../public/install.sh) that is also served from `install.catnip.sh`. This allows user to install with `curl -sSfL install.catnip.sh`.

## Cut a patch release

```shell
just release --patch --push --message="Some exciting new stuff"
```
