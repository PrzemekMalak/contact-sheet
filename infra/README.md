# Infrastructure (AWS CDK)

Two stacks, both in `us-east-1`:

- **`<prefix>Stack`**: a private S3 bucket for full-resolution originals, served only
  through CloudFront (OAC), with an optional custom CDN domain (e.g. `cdn.example.com`)
  and a least-privilege IAM user (`photo-uploader` by default) for the `photo` tool.
- **`<prefix>HostingStack`** (optional): an AWS Amplify app that builds and hosts the
  Hugo site from GitHub, with an optional custom domain.

`<prefix>` is `PhotoSite` unless you change it (below), so the stacks are
`PhotoSiteStack` and `PhotoSiteHostingStack`. The examples use those names and
`example.com`.

You need AWS credentials that can deploy (not the upload key), Python 3 and Node.js (for
`npx`).

## How you run it: `photo cdk`

Your site's settings for these stacks live in your site repo, in the `[cdk]` section of
`contact-sheet.toml`. This repo is only where the infrastructure code comes from: clone it
once, set up its venv, and leave it unchanged, so `git pull` brings updates without
conflicts.

1. Create your site first (`photo init`), so there's a `contact-sheet.toml` to hold the
   settings.
2. Clone this repo and set up the venv once:
   ```bash
   git clone https://github.com/PrzemekMalak/contact-sheet.git
   cd contact-sheet/infra
   python3 -m venv .venv
   .venv/bin/pip install -r requirements.txt
   ```
3. Add the settings to your site's `contact-sheet.toml`:
   ```toml
   [cdk]
   infra = "../contact-sheet/infra"   # this directory: relative to your site, absolute, or ~/...
   profile = "admin"                  # an AWS profile that can deploy; optional

   [cdk.context]                      # passed to CDK as -c key=value
   site_name = "example.com"
   stack_prefix = "PhotoSite"
   ```
4. From your site's root, run CDK through the tool:
   ```bash
   photo cdk diff                      # what would change
   photo cdk deploy PhotoSiteStack     # deploy one stack
   ```

`photo cdk` passes everything after `cdk` to the CDK CLI (`npx aws-cdk@latest`, which
must be at least as new as the `aws-cdk-lib` in the venv) and adds each `[cdk.context]`
value as `-c key=value`; a `-c key=value` you add to the command overrides the file.
`--infra DIR` or the `CONTACT_SHEET_INFRA` environment variable
overrides `infra`, and an exported `AWS_PROFILE` overrides `profile`. It prints the
context keys it passes, never the values.

You can also run `npx aws-cdk ...` in this directory yourself, passing every setting as
`-c key=value` on each command. The `photo cdk` examples below have that same form.

## Site naming

These `[cdk.context]` keys name things:

- `site_name`: used in the CloudFront comment and, slugified, as the Amplify app name.
- `stack_prefix`: CloudFormation stack names are `<prefix>Stack` and `<prefix>HostingStack`.
  Don't change it for a site that's already deployed. A new prefix creates new stacks
  instead of updating the existing ones.
- `uploader_user_name`: the IAM user the `photo` tool uploads with. Changing it on a
  deployed stack replaces the user, so you need a new access key.

Without these keys the defaults are `photo-site` / `PhotoSite` / `photo-uploader`. If you
set `stack_prefix = "Example"`, use `ExampleStack` and `ExampleHostingStack` wherever the
commands below say `PhotoSiteStack` and `PhotoSiteHostingStack`.

## Deploy the originals stack

```bash
# Once per AWS account:
photo cdk bootstrap aws://<ACCOUNT_ID>/us-east-1

photo cdk deploy PhotoSiteStack
```

Without more settings the CDN uses its `*.cloudfront.net` domain. For a custom CDN domain
(ACM certificate + alias), add it to `[cdk.context]`. If your domain is in Route 53, add
the hosted zone too, so DNS validation and the alias record are created automatically:

```toml
[cdk.context]
cdn_domain = "cdn.example.com"
hosted_zone_id = "<ZONE_ID>"
hosted_zone_name = "example.com"
```

If your DNS is **not** in Route53, leave out the `hosted_zone_*` values. The deploy pauses
on the ACM certificate: add the DNS validation CNAME it prints at your DNS provider. Once
the certificate is issued, add a CNAME from `cdn.example.com` to the
`DistributionDomainName` output.

### Price class

The distribution uses CloudFront's `PriceClass_100` by default: it serves only from
edge locations in North America and Europe, so no download of an original costs more
than their per-GB rate. Visitors elsewhere still get the photos, just from a farther
edge. To serve from more locations, set `price_class` to `PriceClass_200` or
`PriceClass_All` in `[cdk.context]`:

```toml
price_class = "PriceClass_All"
```

Changing it on a deployed stack updates the distribution in place.

### Cost alert (optional)

Set `budget_email` and the stack adds an AWS Budgets monthly cost budget of $5 that
emails you when actual costs pass 80% of it and when the forecast for the month passes
100%. Change the amount with `budget_limit` (USD):

```toml
budget_email = "you@example.com"
budget_limit = 10
```

The budget covers the whole AWS account, not just these stacks, so it fits best in an
account used only for the site. It only sends email: it doesn't stop or cap anything, and
cost data reaches it with a delay of up to several hours. Budgets without actions cost
nothing. Deleting the stack deletes the budget. If your site repo is public, so is
everything in `contact-sheet.toml`: leave `budget_email` out of the file and pass it on
the command line instead, e.g. `photo cdk deploy PhotoSiteStack -c budget_email=you@example.com`.

## After deploy

1. Read the stack outputs (`photo cdk deploy` prints them, or use
   `aws cloudformation describe-stacks --stack-name PhotoSiteStack`).
2. Create an access key for the uploader user and store it in your AWS credentials
   (never commit it):
   ```bash
   aws iam create-access-key --user-name photo-uploader
   ```
   Put the key and secret in `~/.aws/credentials` under a named profile, e.g. `[photos]`,
   or in the environment. The `photo` tool uses the standard AWS SDK credential chain.
3. Fill `[s3]` in `contact-sheet.toml` at your site's root from the outputs:
   - `bucket` = `BucketName`
   - `cdn_base` = `https://cdn.example.com` (custom domain) or
     `https://<DistributionDomainName>` (no custom domain)
   - `region` = `us-east-1`
   - `profile` = the profile holding the uploader key, e.g. `photos` (optional)

   Set `bucket` and `cdn_base` together. With a bucket but no `cdn_base`, the tool uploads
   originals but records the local preview as each photo's `original`. `photo rm` then
   can't work out the S3 key, and the original stays in the bucket.
4. From now on `photo add` uploads each original. Photos added earlier in local mode
   stay local: re-adding the same file only adds albums. To upload one, `photo rm <id>`
   and add it again.

The bucket is kept (`RETAIN`) when the stack is deleted, so tearing down the stack
doesn't delete your photos. Delete the bucket by hand if you really mean to.

## Hosting on Amplify

On every push to `main`, Amplify runs the `amplify.yml` at the root of your site's
repository. That file installs a pinned Hugo extended, runs `hugo --minify --gc` and
publishes `public/`. `photo init --amplify` writes it; for a `--module` site it also
installs Go, which Hugo needs to fetch the theme.

The `photo` tool doesn't run in Amplify. You run it on your computer, commit what it
generates, and push. Originals are served from CloudFront (the originals stack above).

### One-time setup

1. Put your site's repository on GitHub and push `main`.
2. Install the **Amplify GitHub App** on your GitHub account. Amplify uses it to read the
   repository and to start a build on every push. The stacks are in us-east-1, so open
   [github.com/apps/aws-amplify-us-east-1/installations/new](https://github.com/apps/aws-amplify-us-east-1/installations/new),
   choose your account, select **Only select repositories** with your site's repository,
   and click **Install**. If you skip this step, the deploy still works: Amplify connects
   the older way, with a webhook and a read-only deploy key on the repository. AWS
   recommends the GitHub App, and an app connected the older way can be migrated later
   in the Amplify console.
3. Create a GitHub
   [personal access token (classic)](https://github.com/settings/tokens/new) with the
   `admin:repo_hook` scope, as
   [the Amplify guide](https://docs.aws.amazon.com/amplify/latest/userguide/setting-up-GitHub-access.html)
   asks. Amplify uses it once, when the stack creates the app, to authorize the GitHub App
   on the repository, and doesn't store it. Store it in AWS Secrets Manager (us-east-1):
   ```bash
   aws secretsmanager create-secret --name github-amplify-token \
     --secret-string '<TOKEN>' --region us-east-1
   ```
   Note the returned ARN.
4. Add the repo, the token secret ARN and the domain to `[cdk.context]`, and deploy the
   hosting stack:
   ```toml
   repository = "https://github.com/<you>/<repo>"
   github_token_secret_arn = "<SECRET_ARN>"
   domain_name = "example.com"
   ```
   ```bash
   photo cdk deploy PhotoSiteHostingStack
   ```
   Leave out `domain_name` to launch without the custom domain first.
5. In the Amplify console (or `aws amplify`), confirm the first build of `main`
   succeeded. The `AmplifyDefaultDomain` output gives the `*.amplifyapp.com` URL.
6. For the custom domain, Amplify provisions an ACM certificate and prints DNS records
   (CNAME/ANAME) to add at your DNS provider for the apex domain and `www`. Add them.
   Once they're verified, the site is live at `https://example.com`.

### Go-live checklist

- [ ] `hugo.toml` set for your site: `baseURL`, `title`, `params.author`, `params.description`.
- [ ] `[cdk]` set in `contact-sheet.toml`: `infra`, `profile`, and `[cdk.context]`
      `site_name`, `stack_prefix`, `uploader_user_name`.
- [ ] Originals stack deployed, bucket + CloudFront live.
- [ ] `contact-sheet.toml` (read by `photo`) filled from the stack outputs: `bucket`,
      `cdn_base`, `region=us-east-1`; uploader access key in your AWS credentials.
- [ ] Albums added locally: `photo add <photos...> -a <album> [--tags ...]` (or use
      `photo gui`). Check that `hugo server` renders them and that the lightbox loads the
      CloudFront original.
- [ ] Committed the new `assets/gallery/**`, `data/**`, `content/**`; pushed to `main`.
- [ ] Hosting stack deployed; first Amplify build of `main` green.
- [ ] DNS records for the apex domain + `www` added and verified; site loads over HTTPS.

## Unit tests (no AWS, no Node)

```bash
.venv/bin/pip install -r requirements-dev.txt
.venv/bin/python -m pytest -q
```
