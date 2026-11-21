import aws_cdk as cdk
from aws_cdk.assertions import Template
from photo_infra.stack import OriginalsStack

def _template(**kwargs) -> Template:
    app = cdk.App()
    stack = OriginalsStack(app, "TestStack",
                            env=cdk.Environment(region="us-east-1"), **kwargs)
    return Template.from_stack(stack)

def test_core_outputs_present():
    tmpl = _template()
    for name in ("BucketName", "DistributionId", "DistributionDomainName", "UploaderUserName"):
        assert tmpl.find_outputs(name), f"missing output {name}"

def test_cdn_domain_output_only_when_set():
    assert not _template().find_outputs("CdnDomain")
    tmpl = _template(cdn_domain="cdn.example.com")
    assert tmpl.find_outputs("CdnDomain")
