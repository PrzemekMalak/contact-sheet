import aws_cdk as cdk
from aws_cdk.assertions import Template, Match
from photo_infra.hosting_stack import HostingStack

DUMMY_SECRET = "arn:aws:secretsmanager:us-east-1:111122223333:secret:gh-token-abc123"

def _template(**kwargs) -> Template:
    app = cdk.App()
    defaults = dict(repository="https://github.com/user/photo-site",
                    github_token_secret_arn=DUMMY_SECRET)
    defaults.update(kwargs)
    stack = HostingStack(app, "TestHosting",
                         env=cdk.Environment(region="us-east-1"), **defaults)
    return Template.from_stack(stack)

def test_amplify_app_created_with_repository():
    _template().has_resource_properties("AWS::Amplify::App", {
        "Repository": "https://github.com/user/photo-site"
    })

def test_access_token_is_a_secretsmanager_reference_not_plaintext():
    tmpl = _template()
    app = tmpl.find_resources("AWS::Amplify::App")
    (props,) = [r["Properties"] for r in app.values()]
    token = props["AccessToken"]
    # must be a CFN dynamic reference to Secrets Manager, never a literal token
    assert isinstance(token, str) and token.startswith("{{resolve:secretsmanager:")

def test_main_branch_has_auto_build():
    _template().has_resource_properties("AWS::Amplify::Branch", {
        "BranchName": "main", "EnableAutoBuild": True
    })

def test_outputs_present():
    tmpl = _template()
    assert tmpl.find_outputs("AmplifyAppId")
    assert tmpl.find_outputs("AmplifyDefaultDomain")

def test_no_domain_when_domain_name_absent():
    _template().resource_count_is("AWS::Amplify::Domain", 0)

def test_domain_maps_root_and_www_to_main():
    tmpl = _template(domain_name="example.com")
    tmpl.has_resource_properties("AWS::Amplify::Domain", {
        "DomainName": "example.com",
        "SubDomainSettings": Match.array_with([
            Match.object_like({"Prefix": "", "BranchName": "main"}),
            Match.object_like({"Prefix": "www", "BranchName": "main"}),
        ]),
    })
