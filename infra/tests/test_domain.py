import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_no_custom_domain_by_default():
    tmpl = _template()
    tmpl.resource_count_is("AWS::CertificateManager::Certificate", 0)
    tmpl.resource_count_is("AWS::Route53::RecordSet", 0)

def test_certificate_and_alias_when_cdn_domain_set():
    tmpl = _template(cdn_domain="cdn.example.com",
                     hosted_zone_id="Z123EXAMPLE", hosted_zone_name="example.com")
    tmpl.has_resource_properties("AWS::CertificateManager::Certificate", {
        "DomainName": "cdn.example.com"
    })
    tmpl.has_resource_properties("AWS::CloudFront::Distribution", {
        "DistributionConfig": Match.object_like({
            "Aliases": ["cdn.example.com"]
        })
    })
    # alias A record created via the hosted zone
    tmpl.resource_count_is("AWS::Route53::RecordSet", 1)

def test_cert_without_zone_still_creates_alias_but_no_record():
    tmpl = _template(cdn_domain="cdn.example.com")
    tmpl.resource_count_is("AWS::CertificateManager::Certificate", 1)
    tmpl.resource_count_is("AWS::Route53::RecordSet", 0)
