package granted

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/common-fate/granted/pkg/cfaws"
	"github.com/common-fate/granted/pkg/securestorage"
	"github.com/urfave/cli/v2"
	"gopkg.in/ini.v1"
)

var SSOCommand = cli.Command{
	Name:        "sso",
	Usage:       "Manage your local AWS configuration file from information available in AWS SSO",
	Subcommands: []*cli.Command{&GenerateCommand, &PopulateCommand},
}

var GenerateCommand = cli.Command{
	Name:      "generate",
	Usage:     "Prints an AWS configuration file to stdout with profiles from accounts and roles available in AWS SSO",
	UsageText: "granted [global options] sso generate [command options] [sso-start-url]",
	Flags:     []cli.Flag{&cli.StringFlag{Name: "prefix", Usage: "Specify a prefix for all generated profile names"}, &cli.StringFlag{Name: "region", Usage: "Specify the SSO region", DefaultText: "us-east-1"}},
	Action: func(c *cli.Context) error {
		options, err := parseCliOptions(c)
		if err != nil {
			return err
		}
		ssoProfiles, err := listSSOProfiles(c.Context, ListSSOProfilesInput{
			StartUrl:  options.StartUrl,
			SSORegion: options.SSORegion,
		})
		if err != nil {
			return err
		}
		config := ini.Empty()
		err = mergeSSOProfiles(config, options.Prefix, ssoProfiles)
		if err != nil {
			return err
		}
		_, err = config.WriteTo(os.Stdout)
		return err
	},
}

var PopulateCommand = cli.Command{
	Name:      "populate",
	Usage:     "Populate your local AWS configuration file with profiles from accounts and roles available in AWS SSO",
	UsageText: "granted [global options] sso populate [command options] [sso-start-url]",
	Flags:     []cli.Flag{&cli.StringFlag{Name: "prefix", Usage: "Specify a prefix for all generated profile names"}, &cli.StringFlag{Name: "region", Usage: "Specify the SSO region", DefaultText: "us-east-1"}},
	Action: func(c *cli.Context) error {
		options, err := parseCliOptions(c)
		if err != nil {
			return err
		}

		ssoProfiles, err := listSSOProfiles(c.Context, ListSSOProfilesInput{
			StartUrl:  options.StartUrl,
			SSORegion: options.SSORegion,
		})
		if err != nil {
			return err
		}

		configFilename := config.DefaultSharedConfigFilename()

		config, err := ini.LoadSources(ini.LoadOptions{
			AllowNonUniqueSections:  false,
			SkipUnrecognizableLines: false,
		}, configFilename)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			config = ini.Empty()
		}
		if err := mergeSSOProfiles(config, options.Prefix, ssoProfiles); err != nil {
			return err
		}

		err = config.SaveTo(configFilename)
		if err != nil {
			return err
		}
		return nil
	},
}

func parseCliOptions(c *cli.Context) (*SSOCommonOptions, error) {
	prefix := c.String("prefix")
	match, err := regexp.MatchString("^[A-Za-z0-9_-]*$", prefix)
	if err != nil {
		return nil, err
	}

	if !match {
		return nil, fmt.Errorf("--prefix flag must be alpha-numeric, underscores or hyphens")
	}

	ssoRegion, err := cfaws.ExpandRegion(c.String("region"))
	if err != nil {
		return nil, err
	}

	if c.Args().Len() != 1 {
		return nil, fmt.Errorf("please provide an sso start url")
	}

	startUrl := c.Args().First()

	options := SSOCommonOptions{
		Prefix:    prefix,
		StartUrl:  startUrl,
		SSORegion: ssoRegion,
	}

	return &options, nil
}

type SSOCommonOptions struct {
	Prefix    string
	StartUrl  string
	SSORegion string
}

type ListSSOProfilesInput struct {
	SSORegion string
	StartUrl  string
}

type SSOProfile struct {
	// SSO details
	StartUrl  string
	SSORegion string
	// Account and role details
	AccountId   string
	AccountName string
	RoleName    string
}

func listSSOProfiles(ctx context.Context, input ListSSOProfilesInput) ([]SSOProfile, error) {
	cfg := aws.NewConfig()
	cfg.Region = input.SSORegion
	secureSSOTokenStorage := securestorage.NewSecureSSOTokenStorage()
	ssoToken := secureSSOTokenStorage.GetValidSSOToken(input.StartUrl)
	var err error
	if ssoToken == nil {
		ssoToken, err = cfaws.SSODeviceCodeFlowFromStartUrl(ctx, *cfg, input.StartUrl)
		if err != nil {
			return nil, err
		}
	}

	ssoClient := sso.NewFromConfig(*cfg)

	var ssoProfiles []SSOProfile

	listAccountsNextToken := ""
	for {
		listAccountsInput := sso.ListAccountsInput{AccessToken: &ssoToken.AccessToken}
		if listAccountsNextToken != "" {
			listAccountsInput.NextToken = &listAccountsNextToken
		}

		listAccountsOutput, err := ssoClient.ListAccounts(ctx, &listAccountsInput)
		if err != nil {
			return nil, err
		}

		for _, account := range listAccountsOutput.AccountList {
			listAccountRolesNextToken := ""
			for {
				listAccountRolesInput := sso.ListAccountRolesInput{
					AccessToken: &ssoToken.AccessToken,
					AccountId:   account.AccountId,
				}
				if listAccountRolesNextToken != "" {
					listAccountRolesInput.NextToken = &listAccountRolesNextToken
				}

				listAccountRolesOutput, err := ssoClient.ListAccountRoles(ctx, &listAccountRolesInput)
				if err != nil {
					return nil, err
				}

				for _, role := range listAccountRolesOutput.RoleList {
					ssoProfiles = append(ssoProfiles, SSOProfile{
						StartUrl:    input.StartUrl,
						SSORegion:   input.SSORegion,
						AccountId:   *role.AccountId,
						AccountName: *account.AccountName,
						RoleName:    *role.RoleName,
					})
				}

				if listAccountRolesOutput.NextToken == nil {
					break
				}

				listAccountRolesNextToken = *listAccountRolesOutput.NextToken
			}
		}

		if listAccountsOutput.NextToken == nil {
			break
		}

		listAccountsNextToken = *listAccountsOutput.NextToken
	}

	return ssoProfiles, nil
}

func mergeSSOProfiles(config *ini.File, prefix string, ssoProfiles []SSOProfile) error {
	for _, ssoProfile := range ssoProfiles {
		sectionName := "profile " + prefix + normalizeAccountName(ssoProfile.AccountName) + "/" + ssoProfile.RoleName

		config.DeleteSection(sectionName)
		section, err := config.NewSection(sectionName)
		if err != nil {
			return err
		}
		err = section.ReflectFrom(&struct {
			SSOStartURL  string `ini:"sso_start_url"`
			SSORegion    string `ini:"sso_region"`
			SSOAccountID string `ini:"sso_account_id"`
			SSORoleName  string `ini:"sso_role_name"`
		}{
			SSOStartURL:  ssoProfile.StartUrl,
			SSORegion:    ssoProfile.SSORegion,
			SSOAccountID: ssoProfile.AccountId,
			SSORoleName:  ssoProfile.RoleName,
		})
		if err != nil {
			return err
		}

	}

	return nil
}

func normalizeAccountName(accountName string) string {
	return strings.ReplaceAll(accountName, " ", "-")
}
