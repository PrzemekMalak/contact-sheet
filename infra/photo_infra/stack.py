from __future__ import annotations
from constructs import Construct
from aws_cdk import (Stack, RemovalPolicy, CfnOutput,
                     aws_s3 as s3,
                     aws_cloudfront as cloudfront,
                     aws_cloudfront_origins as origins,
                     aws_certificatemanager as acm,
                     aws_route53 as route53,
                     aws_route53_targets as route53_targets,
                     aws_iam as iam,
                     aws_budgets as budgets)

class OriginalsStack(Stack):
    def __init__(self, scope: Construct, construct_id: str, *,
                 site_name: str = "photo-site",
                 cdn_domain: str | None = None,
                 hosted_zone_id: str | None = None,
                 hosted_zone_name: str | None = None,
                 originals_prefix: str = "originals",
                 uploader_user_name: str = "photo-uploader",
                 price_class: cloudfront.PriceClass = cloudfront.PriceClass.PRICE_CLASS_100,
                 budget_email: str | None = None,
                 budget_limit: float = 5,
                 **kwargs) -> None:
        super().__init__(scope, construct_id, **kwargs)
        self.originals_prefix = originals_prefix
        self.bucket = s3.Bucket(
            self, "OriginalsBucket",
            block_public_access=s3.BlockPublicAccess.BLOCK_ALL,
            encryption=s3.BucketEncryption.S3_MANAGED,
            enforce_ssl=True,
            versioned=False,
            removal_policy=RemovalPolicy.RETAIN,
        )
        certificate = None
        domain_names = None
        zone = None
        if cdn_domain:
            if hosted_zone_id and hosted_zone_name:
                zone = route53.HostedZone.from_hosted_zone_attributes(
                    self, "Zone",
                    hosted_zone_id=hosted_zone_id, zone_name=hosted_zone_name)
                validation = acm.CertificateValidation.from_dns(zone)
            else:
                validation = acm.CertificateValidation.from_dns()
            certificate = acm.Certificate(
                self, "CdnCert", domain_name=cdn_domain, validation=validation)
            domain_names = [cdn_domain]
        self.distribution = cloudfront.Distribution(
            self, "Distribution",
            comment=f"{site_name} originals CDN",
            default_behavior=cloudfront.BehaviorOptions(
                origin=origins.S3BucketOrigin.with_origin_access_control(self.bucket),
                viewer_protocol_policy=cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
                allowed_methods=cloudfront.AllowedMethods.ALLOW_GET_HEAD,
                cache_policy=cloudfront.CachePolicy.CACHING_OPTIMIZED,
            ),
            domain_names=domain_names,
            certificate=certificate,
            # PriceClass_100 serves from North America and Europe only, so no
            # download costs more than their per-GB rate.
            price_class=price_class,
        )
        if cdn_domain and zone is not None:
            route53.ARecord(
                self, "CdnAlias",
                zone=zone,
                record_name=cdn_domain,
                target=route53.RecordTarget.from_alias(
                    route53_targets.CloudFrontTarget(self.distribution)),
            )
        self.cdn_domain = cdn_domain
        self.uploader = iam.User(self, "Uploader", user_name=uploader_user_name)
        self.uploader.add_to_policy(iam.PolicyStatement(
            actions=["s3:ListBucket"],
            resources=[self.bucket.bucket_arn],
        ))
        self.uploader.add_to_policy(iam.PolicyStatement(
            actions=["s3:PutObject", "s3:DeleteObject"],
            resources=[self.bucket.arn_for_objects(f"{originals_prefix}/*")],
        ))
        if budget_email:
            # A monthly cost budget for the whole account, not just this stack.
            # It only sends email; it never stops or caps anything.
            subscribers = [budgets.CfnBudget.SubscriberProperty(
                subscription_type="EMAIL", address=budget_email)]
            def alert(kind: str, percent: int):
                return budgets.CfnBudget.NotificationWithSubscribersProperty(
                    notification=budgets.CfnBudget.NotificationProperty(
                        notification_type=kind, comparison_operator="GREATER_THAN",
                        threshold=percent, threshold_type="PERCENTAGE"),
                    subscribers=subscribers)
            budgets.CfnBudget(
                self, "MonthlyBudget",
                budget=budgets.CfnBudget.BudgetDataProperty(
                    budget_name=f"{site_name} monthly cost",
                    budget_type="COST",
                    time_unit="MONTHLY",
                    budget_limit=budgets.CfnBudget.SpendProperty(
                        amount=budget_limit, unit="USD"),
                ),
                notifications_with_subscribers=[
                    alert("ACTUAL", 80),
                    alert("FORECASTED", 100),
                ],
            )
        CfnOutput(self, "BucketName", value=self.bucket.bucket_name)
        CfnOutput(self, "DistributionId", value=self.distribution.distribution_id)
        CfnOutput(self, "DistributionDomainName",
                  value=self.distribution.distribution_domain_name)
        CfnOutput(self, "UploaderUserName", value=self.uploader.user_name)
        if cdn_domain:
            CfnOutput(self, "CdnDomain", value=cdn_domain)
