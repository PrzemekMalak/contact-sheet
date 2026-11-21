from __future__ import annotations
from constructs import Construct
from aws_cdk import Stack, CfnOutput, SecretValue, aws_amplify as amplify

class HostingStack(Stack):
    def __init__(self, scope: Construct, construct_id: str, *,
                 repository: str,
                 app_name: str = "photo-site",
                 github_token_secret_arn: str,
                 domain_name: str | None = None,
                 branch_name: str = "main",
                 **kwargs) -> None:
        super().__init__(scope, construct_id, **kwargs)

        access_token = SecretValue.secrets_manager(github_token_secret_arn).unsafe_unwrap()

        self.app = amplify.CfnApp(
            self, "GalleryApp",
            name=app_name,
            repository=repository,
            access_token=access_token,
            platform="WEB",
        )
        self.branch = amplify.CfnBranch(
            self, "MainBranch",
            app_id=self.app.attr_app_id,
            branch_name=branch_name,
            enable_auto_build=True,
            stage="PRODUCTION",
        )

        if domain_name:
            self.domain = amplify.CfnDomain(
                self, "Domain",
                app_id=self.app.attr_app_id,
                domain_name=domain_name,
                sub_domain_settings=[
                    amplify.CfnDomain.SubDomainSettingProperty(
                        prefix="", branch_name=branch_name),
                    amplify.CfnDomain.SubDomainSettingProperty(
                        prefix="www", branch_name=branch_name),
                ],
            )
            self.domain.add_dependency(self.branch)

        CfnOutput(self, "AmplifyAppId", value=self.app.attr_app_id)
        CfnOutput(self, "AmplifyDefaultDomain", value=self.app.attr_default_domain)
