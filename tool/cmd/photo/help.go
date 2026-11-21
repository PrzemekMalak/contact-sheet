package main

import (
	"io"
	"os"
)

// helpOut is where -h output goes (a variable so tests can capture it).
var helpOut io.Writer = os.Stdout

// commands lists every photo command; docs/cli.md has a section for each.
var commands = []string{
	"init", "theme", "add", "link", "unlink", "retag", "rm", "rm-album",
	"sync", "backup", "gui", "cdk", "version",
}

// commandHelp is printed by `photo <command> -h`. It needs no site, so it
// works anywhere. Keep it in step with the flags in main.go and docs/cli.md.
var commandHelp = map[string]string{
	"init": `usage: photo init <dir> [--title T] [--url URL] [--author A] [--amplify] [--module]

Create a new site in <dir> (missing or empty) and download the theme into
themes/contact-sheet/.

  --title T     site title (default: the directory name)
  --url URL     site URL, e.g. https://photos.example.com/ (default: https://example.com/)
  --author A    name in the footer (default: the title)
  --amplify     also write amplify.yml for AWS Amplify hosting
  --module      import the theme as a Hugo Module (needs Go) instead of copying it
`,
	"theme": `usage: photo theme update [--version vX.Y.Z]

Replace the theme copy in themes/contact-sheet/ with a release. Run from the
site root. A site that imports the theme as a Hugo Module uses
"hugo mod get -u" instead.

  --version V   theme release to install (default: latest)
`,
	"add": `usage: photo add <paths...> [-a album]... [--tags a,b] [--featured] [--no-exif] [-r]

Import photos: make the WebP thumbnail and preview, read EXIF, upload the
original (when [s3] bucket is set), and update data/photos.json and the album,
tag and gear pages. A photo that's already in the catalog only gains albums.

  -a album      add to this album (repeatable; without -a: Unassigned)
  --tags a,b    comma-separated tags
  --featured    mark as featured
  --no-exif     don't read EXIF
  -r            recurse into directories (picks .jpg and .jpeg files)
`,
	"link": `usage: photo link <album> <id...>

Add photos to an album, creating its page if needed.
`,
	"unlink": `usage: photo unlink <album> <id...>

Remove photos from one album. The photos and their files stay.
`,
	"retag": `usage: photo retag <id> --tags a,b

Replace a photo's tags. --tags "" removes them all.

  --tags a,b    comma-separated tags
`,
	"rm": `usage: photo rm <id...> [--yes]

Delete photos entirely: catalog entries, thumbnails and previews, and the
originals in S3. There is no confirmation; --yes is accepted and ignored.
`,
	"rm-album": `usage: photo rm-album <album> [--purge-orphans] [--yes]

Remove every photo from the album and delete its page. Photos left in no
album move to Unassigned.

  --purge-orphans   also delete the photos left in no album (like photo rm)
  --yes             accepted and ignored; there is no confirmation
`,
	"sync": `usage: photo sync

Re-sort data/photos.json and rebuild the tag and gear pages. Run it after
editing the catalog by hand.
`,
	"backup": `usage: photo backup <dir> [--jobs N]

Download the original of every photo in the catalog from the CDN into <dir>.
Needs no AWS credentials. Incremental: a file already in <dir> is downloaded
again only if its size on the CDN changed; nothing is ever deleted.

  --jobs N      parallel downloads (default: 4)
`,
	"gui": `usage: photo gui [--port N]

Start the local photo manager at http://127.0.0.1:N and open it.

  --port N      port to listen on, 127.0.0.1 only (default: 8765)
`,
	"cdk": `usage: photo cdk [--infra DIR] <cdk arguments...>

Run the AWS CDK CLI in contact-sheet's infra/ with the [cdk] settings from
contact-sheet.toml. Everything after cdk goes to CDK; a -c key=value you add
overrides the file. "photo cdk deploy -h" shows CDK's own help.

  --infra DIR   contact-sheet's infra/ directory (overrides CONTACT_SHEET_INFRA
                and [cdk] infra); not passed on to CDK

  photo cdk diff                      what would change
  photo cdk deploy PhotoSiteStack     deploy one stack (use your stack_prefix)
  photo cdk synth                     print the CloudFormation templates
`,
	"version": `usage: photo version

Print the tool version.
`,
}

func isHelpFlag(a string) bool { return a == "-h" || a == "-help" || a == "--help" }

// wantsHelp reports whether args ask for the command's help. For cdk only a
// help flag right after "cdk" counts; later ones belong to the CDK CLI.
func wantsHelp(cmd string, args []string) bool {
	if cmd == "cdk" {
		return len(args) > 0 && isHelpFlag(args[0])
	}
	for _, a := range args {
		if isHelpFlag(a) {
			return true
		}
	}
	return false
}
