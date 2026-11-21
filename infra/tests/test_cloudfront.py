import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from aws_cdk import aws_cloudfront as cloudfront
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_one_distribution_exists():
    _template().resource_count_is("AWS::CloudFront::Distribution", 1)

def test_uses_origin_access_control_not_oai():
    tmpl = _template()
    tmpl.resource_count_is("AWS::CloudFront::OriginAccessControl", 1)
    # No legacy Origin Access Identity
    tmpl.resource_count_is("AWS::CloudFront::CloudFrontOriginAccessIdentity", 0)

def test_viewer_protocol_redirects_to_https():
    _template().has_resource_properties("AWS::CloudFront::Distribution", {
        "DistributionConfig": Match.object_like({
            "DefaultCacheBehavior": Match.object_like({
                "ViewerProtocolPolicy": "redirect-to-https"
            })
        })
    })

def test_bucket_policy_grants_cloudfront_service_principal():
    _template().has_resource_properties("AWS::S3::BucketPolicy", {
        "PolicyDocument": Match.object_like({
            "Statement": Match.array_with([
                Match.object_like({
                    "Principal": {"Service": "cloudfront.amazonaws.com"}
                })
            ])
        })
    })

def _price_class(tmpl: Template) -> str:
    dists = tmpl.find_resources("AWS::CloudFront::Distribution")
    (dist,) = dists.values()
    return dist["Properties"]["DistributionConfig"]["PriceClass"]

def test_price_class_defaults_to_100():
    # North America and Europe edges only, so a download never costs more
    # than their per-GB rate.
    assert _price_class(_template()) == "PriceClass_100"

def test_price_class_can_be_widened():
    assert _price_class(_template(price_class=cloudfront.PriceClass.PRICE_CLASS_ALL)) == "PriceClass_All"
