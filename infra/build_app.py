from __future__ import annotations
import re
import aws_cdk as cdk
from aws_cdk import aws_cloudfront as cloudfront
from photo_infra.stack import OriginalsStack
from photo_infra.hosting_stack import HostingStack

REGION = "us-east-1"
# Generic defaults; a deployment sets its own in cdk.json "context" (or -c).
DEFAULT_SITE_NAME = "photo-site"
DEFAULT_STACK_PREFIX = "PhotoSite"
# Context value -> CloudFront price class. The keys are the CloudFormation names.
PRICE_CLASSES = {
    "PriceClass_100": cloudfront.PriceClass.PRICE_CLASS_100,
    "PriceClass_200": cloudfront.PriceClass.PRICE_CLASS_200,
    "PriceClass_All": cloudfront.PriceClass.PRICE_CLASS_ALL,
}
DEFAULT_PRICE_CLASS = "PriceClass_100"
DEFAULT_BUDGET_LIMIT = 5.0  # USD a month; used only when budget_email is set

def _budget_limit(value) -> float:
    if value is None or value == "":
        return DEFAULT_BUDGET_LIMIT
    try:
        limit = float(value)
    except (TypeError, ValueError):
        limit = 0.0
    if not limit > 0:
        raise ValueError(f"Context 'budget_limit' must be a positive number of USD, got {value!r}.")
    return limit

def build(app: cdk.App) -> None:
    ctx = app.node.try_get_context
    site_name = ctx("site_name") or DEFAULT_SITE_NAME
    # Stack IDs are the CloudFormation stack names: changing the prefix of a
    # deployed site creates new stacks instead of updating the existing ones.
    prefix = ctx("stack_prefix") or DEFAULT_STACK_PREFIX
    price_class = ctx("price_class") or DEFAULT_PRICE_CLASS
    if price_class not in PRICE_CLASSES:
        raise ValueError(f"Unknown context 'price_class': {price_class!r}. Use one of: {', '.join(PRICE_CLASSES)}.")
    OriginalsStack(
        app, f"{prefix}Stack",
        site_name=site_name,
        uploader_user_name=ctx("uploader_user_name") or "photo-uploader",
        cdn_domain=ctx("cdn_domain"),
        hosted_zone_id=ctx("hosted_zone_id"),
        hosted_zone_name=ctx("hosted_zone_name"),
        price_class=PRICE_CLASSES[price_class],
        budget_email=ctx("budget_email"),
        budget_limit=_budget_limit(ctx("budget_limit")),
        env=cdk.Environment(region=REGION),
    )
    repository = ctx("repository")
    if repository:
        github_token_secret_arn = ctx("github_token_secret_arn")
        if not github_token_secret_arn:
            raise ValueError("HostingStack requires context 'github_token_secret_arn' when 'repository' is set (pass -c github_token_secret_arn=<secret ARN>).")
        HostingStack(
            app, f"{prefix}HostingStack",
            app_name=re.sub(r"[^A-Za-z0-9]+", "-", site_name).strip("-"),
            repository=repository,
            github_token_secret_arn=github_token_secret_arn,
            domain_name=ctx("domain_name"),
            branch_name=ctx("branch_name") or "main",
            env=cdk.Environment(region=REGION),
        )
