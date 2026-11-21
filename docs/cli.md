# The `photo` command line

Reference for every `photo` command and for `contact-sheet.toml`. For the photo manager in
the browser, see the [photo manager guide](gui.md).

Run `photo` from your site's root, the folder with `contact-sheet.toml`. The exceptions
are `photo init`, which creates that folder, and `photo version`. `photo <command> -h`
prints a command's options anywhere.

Flags can come before or after the other arguments: `photo add -a iceland *.jpg` and
`photo add *.jpg -a iceland` are the same.

**Photo ids.** A photo's id is its file name without the extension, lowercased, with
spaces and other separators turned into `-`: `IMG 001.jpg` becomes `img-001`. Commands
that take `<id...>` use these ids. Two files with the same name get the same id, so the
second one is treated as already added.

**What the tool writes**, with the default paths from `contact-sheet.toml`:

| Path | What it is |
| --- | --- |
| `data/photos.json` | the catalog: one record per photo with albums, tags, caption, EXIF |
| `assets/gallery/<id>-thumb.webp` | thumbnail, longest edge 400px |
| `assets/gallery/<id>-preview.webp` | preview for the lightbox, longest edge 1600px |
| `content/albums/<album>/index.md` | album page (title, cover); yours to edit |
| `content/tags/<tag>.md` | tag page, rebuilt from the catalog |
| `content/gear/<slug>/index.md` | camera, lens and camera + lens pages; you can edit `title` |
| `originals/<id>.jpg` in the S3 bucket | the original (its extension, lowercased), when `[s3] bucket` is set |

Everything except the S3 original is a file in your git working tree: review it with
`git status` and undo it with git until you commit.

## Site

### `photo init`

```
photo init <dir> [--title T] [--url URL] [--author A] [--amplify] [--module]
```

Creates a new site in `<dir>`, which must be missing or empty: `hugo.toml`,
`contact-sheet.toml`, `content/_index.md`, `content/about.md` and `.gitignore`. Then it
downloads the theme release matching the tool's version into `themes/contact-sheet/`. If
the download fails, it removes what it created.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--title T` | the directory name | site title in `hugo.toml` |
| `--url URL` | `https://example.com/` | `baseURL` in `hugo.toml` |
| `--author A` | the title | name in the footer |
| `--amplify` | off | also write `amplify.yml`, the build for AWS Amplify Hosting |
| `--module` | off | import the theme as a Hugo Module (needs Go) and write `go.mod`, instead of downloading a copy |

`photo init` doesn't create a git repository or touch AWS.

### `photo theme`

```
photo theme update [--version vX.Y.Z]
```

Replaces the theme copy in `themes/contact-sheet/` with a release (`--version`, default
`latest`) and prints the old and new version. A site created with `--module` updates the
theme with `hugo mod get -u` instead. Your own overrides in `layouts/` and `assets/` are
not touched.

## Photos

### `photo add`

```
photo add <paths...> [-a album]... [--tags a,b] [--featured] [--no-exif] [-r]
```

Imports photos. For each new photo it:

1. writes the thumbnail and preview to `assets/gallery/`,
2. reads EXIF (camera, lens, exposure, date taken),
3. uploads the original to S3 when `[s3] bucket` is set, with its `Content-Type` and
   `Cache-Control: public, max-age=604800` (a week); in local mode the lightbox uses the
   preview instead,
4. adds a record to `data/photos.json`,

and then creates the album pages and rebuilds the tag and gear pages.

A photo whose id is already in the catalog is not processed again: it only gains the
albums you pass with `-a`. Its tags and featured flag stay as they are.

| Flag | Meaning |
| --- | --- |
| `-a album` | add to this album; repeat for several. Without `-a`, photos land in Unassigned |
| `--tags a,b` | comma-separated tags |
| `--featured` | mark as featured (the homepage can show the featured photos) |
| `--no-exif` | don't read EXIF |
| `-r` | recurse into directories; picks `.jpg` and `.jpeg` files. Without `-r`, a directory is an error |

Album names are used as folder names: they can't contain `/` or `\`, and `unassigned`
is reserved.

### `photo link`

```
photo link <album> <id...>
```

Adds photos to an album, creating `content/albums/<album>/index.md` if it doesn't exist.

### `photo unlink`

```
photo unlink <album> <id...>
```

Removes photos from one album. The photos, their files and their other albums stay. A
photo left in no album shows up in Unassigned.

### `photo retag`

```
photo retag <id> --tags a,b
```

Replaces a photo's tags and rebuilds the tag pages. `--tags ""` removes them all.

### `photo rm`

```
photo rm <id...> [--yes]
```

Deletes photos entirely: their records, thumbnails and previews, and their originals in
S3. It doesn't ask for confirmation (`--yes` is accepted and ignored). The S3 delete runs
right away, so the original can't be recovered from git. The [photo manager](gui.md)
instead waits until a week after the change is published. Deletes the photo manager
queued and hasn't published yet are left alone.

### `photo rm-album`

```
photo rm-album <album> [--purge-orphans] [--yes]
```

Removes every photo from the album and deletes the album page. Prints how many photos
were unlinked, how many were left in no album (orphans), and how many were deleted.

| Flag | Meaning |
| --- | --- |
| `--purge-orphans` | also delete the orphans, like `photo rm` |
| `--yes` | accepted and ignored; there's no confirmation |

The [photo manager](gui.md) does the same with **Delete album**, asks first, and deletes
S3 originals a week after you publish.

### `photo sync`

```
photo sync
```

Re-sorts `data/photos.json` and rebuilds the tag and gear pages. Run it after you edit
the catalog by hand, or to refresh gear pages after a theme or tool update.

## Originals

### `photo backup`

```
photo backup <dir> [--jobs N]
```

Downloads the original of every photo in the catalog from the CDN (`[s3] cdn_base`) into
`<dir>`. It uses the public URLs, so it needs no AWS credentials. It copies what the
catalog lists, not the whole bucket: an object no photo uses isn't copied.

It's incremental. A file already in `<dir>` is checked with a HEAD request and downloaded
again only if its size changed (another photo got the same file name); if the CDN doesn't
report a size, the file is kept. A download goes to a temporary file first, so an
interrupted one never looks complete, and nothing in `<dir>` is ever deleted. It also saves
a copy of the catalog as `<dir>/photos.json`. Photos added in local mode have no original
on the CDN; they're counted as local-only, because their previews are in git.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--jobs N` | 4 | parallel downloads |

Failed downloads are listed at the end; run the command again to retry them.

For a full copy of the bucket, for example to restore after losing it, use the AWS CLI
with credentials that can read the bucket (the upload key can't):

```bash
aws s3 sync s3://<BucketName> ./originals-copy --profile <read-profile>
```

## Photo manager

### `photo gui`

```
photo gui [--port N]
```

Starts the photo manager at `http://127.0.0.1:8765` (or `--port N`) and opens it in your
browser. It listens on `127.0.0.1` only. See the [photo manager guide](gui.md).

## AWS

### `photo cdk`

```
photo cdk [--infra DIR] <cdk arguments...>
```

Runs the AWS CDK CLI (`npx aws-cdk@latest`) in contact-sheet's `infra/` directory, with
the settings from the `[cdk]` section of `contact-sheet.toml`. Everything after `cdk` goes
to CDK, and each `[cdk.context]` value is passed as `-c key=value`. A `-c key=value` you
add to the command overrides the file.

```bash
photo cdk diff                      # what would change
photo cdk deploy PhotoSiteStack     # deploy one stack (use your stack_prefix)
photo cdk synth                     # print the CloudFormation templates
photo cdk deploy PhotoSiteStack -c price_class=PriceClass_All
```

| Flag | Meaning |
| --- | --- |
| `--infra DIR` | the `infra/` directory of your contact-sheet clone, anywhere in the command (it isn't passed on to CDK). Overrides `CONTACT_SHEET_INFRA` and `[cdk] infra` |

Before it runs, it checks that the directory has `cdk.json`, `app.py` and the Python venv,
and that `npx` is installed. It prints the infra directory, the AWS profile and the
context keys (not their values). The exit code is CDK's. `photo cdk -h` prints this help;
`photo cdk deploy -h` shows CDK's own. Setup and the available settings are in
[infra/README.md](../infra/README.md).

## Other

### `photo version`

```
photo version
```

Prints the tool version (e.g. `v0.3.4`, or `dev` for a local build).

## `contact-sheet.toml`

`photo init` writes this file with the values below. If it's missing, the tool reads the
older name `mygallery.toml`.

### `[project]`: where files go

| Key | Value from `photo init` | Meaning |
| --- | --- | --- |
| `assets_dir` | `"assets/gallery"` | thumbnails and previews |
| `photos_index` | `"data/photos.json"` | the catalog |
| `albums_dir` | `"content/albums"` | album pages |
| `tags_dir` | `"content/tags"` | tag pages |

The theme expects these paths, so change them only together with your own templates.

### `[images]`: derivatives

| Key | Value from `photo init` | Meaning |
| --- | --- | --- |
| `thumb_max_edge` | `400` | thumbnail's longest edge, in pixels |
| `preview_max_edge` | `1600` | preview's longest edge, in pixels |
| `webp_quality` | `82` | WebP quality, 0–100 |

Changes apply to photos you add afterwards. Existing derivatives stay as they are.

### `[s3]`: originals

| Key | Value from `photo init` | Meaning |
| --- | --- | --- |
| `bucket` | `""` | S3 bucket for originals. Empty means local mode: nothing is uploaded |
| `originals_prefix` | `"originals"` | key prefix in the bucket. Don't change it after uploading |
| `cdn_base` | `""` | public base URL of the originals, e.g. `https://cdn.example.com` |
| `region` | `"us-east-1"` | the bucket's region |
| `profile` | not set | AWS profile with the upload key. Unset: the default credential chain |

Set `bucket` and `cdn_base` together, from the outputs of the originals stack.

### `[cdk]`: AWS stacks

Read only by `photo cdk`. `photo init` writes it commented out.

| Key | Meaning |
| --- | --- |
| `infra` | your contact-sheet clone's `infra/` directory: relative to the site root, absolute, or starting with `~/` |
| `profile` | an AWS profile that can deploy (not the upload key). A non-empty exported `AWS_PROFILE` wins |
| `[cdk.context]` | values passed to CDK as `-c key=value`: strings, numbers or booleans. A value starting with `TODO` stops the command until you fill it in or delete it |

The context keys (`site_name`, `stack_prefix`, `cdn_domain`, `price_class`, `budget_email`
and the rest) are described in [infra/README.md](../infra/README.md).
