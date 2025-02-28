package registry

import (
	"os"
	"testing"

	"gopkg.in/ini.v1"
)

func TestGetNonGrantedProfiles(t *testing.T) {

	tests := []struct {
		name              string
		want              []string
		configFileContent string
	}{
		{
			name:              "with autogenerated profiles ok",
			want:              []string{"profile before.1", "profile before.2", "profile after.1", "profile after.2"},
			configFileContent: ConfigWithGeneratedSections,
		},
		{
			name:              "without autogenerated profiles ok",
			want:              []string{"profile one", "profile two", "profile three"},
			configFileContent: configWithoutGeneratedSections,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := ini.Load([]byte(tt.configFileContent))
			if err != nil {
				t.Fatal(err)
			}

			want := tt.want
			gotSections := getNonGrantedProfiles(file)

			var got []string
			for _, sec := range gotSections {
				got = append(got, sec.Name())
			}

			if len(want) != len(got) {
				t.Errorf("got %v, want %v", got, want)
			}
			for i, v := range got {
				if v != want[i] {
					t.Errorf("invalid key %v", v)
				}
			}

		})
	}
}

func TestGetGrantedGeneratedSections(t *testing.T) {

	tests := []struct {
		name              string
		want              []string
		configFileContent string
	}{
		{
			name:              "with autogenerated profiles ok",
			want:              []string{"granted_registry_start https://github.com/octo/repo_one.git", "granted_registry_end https://github.com/octo/repo_one.git", "granted_registry_end https://github.com/octo/repo_two.git", "granted_registry_start https://github.com/octo/repo_two.git", "profile s1.one", "profile s1.two", "profile s2.one", "profile s2.two", "profile s2.three", "duplicate"},
			configFileContent: ConfigWithGeneratedSections,
		},
		{
			name:              "without autogenerated profiles ok",
			want:              []string{},
			configFileContent: configWithoutGeneratedSections,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := ini.Load([]byte(tt.configFileContent))
			if err != nil {
				t.Fatal(err)
			}

			want := tt.want
			gotSections := getGrantedGeneratedSections(file)

			var got []string
			for _, sec := range gotSections {
				got = append(got, sec.Name())
			}

			if len(want) != len(got) {
				t.Errorf("got %v, want %v", got, want)
			}

		})
	}
}

func TestRemoveAutogeneratedProfiles(t *testing.T) {
	tests := []struct {
		name              string
		want              []string
		configFileContent string
	}{
		{
			name:              "with autogenerated profiles ok",
			configFileContent: ConfigWithGeneratedSections,
		},
		{
			name:              "without autogenerated profiles ok",
			configFileContent: configWithoutGeneratedSections,
		},
	}

	tmpDir := os.TempDir()
	tmp, err := os.CreateTemp(tmpDir, "config_file")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(tmp.Name())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ini.Load([]byte(tt.configFileContent))
			if err != nil {
				t.Fatal(err)
			}

			if _, err = f.WriteTo(tmp); err != nil {
				t.Fatal(err)
			}

			if err := removeAutogeneratedProfiles(f, tmp.Name()); err != nil {
				t.Error(err)
			}

			file, err := ini.Load(tmp.Name())
			if err != nil {
				t.Error(err)
			}

			grantedProfiles := getGrantedGeneratedSections(file)

			if len(grantedProfiles) != 0 {
				t.Errorf("Expected no profiles. Got %v", grantedProfiles)
			}

		})

	}

}

func TestGetGeneratedSectionByRegistryURL(t *testing.T) {
	tests := []struct {
		name              string
		want              []string
		repoURL           string
		configFileContent string
	}{
		{
			name:              "with autogenerated profiles ok",
			repoURL:           "https://github.com/octo/repo_two.git",
			configFileContent: ConfigWithGeneratedSections,
			want:              []string{"profile s2.one", "profile s2.two", "profile s2.three", "granted_registry_start https://github.com/octo/repo_two.git", "granted_registry_end https://github.com/octo/repo_two.git"},
		},
		{
			name:              "without autogenerated profiles ok",
			repoURL:           "https://github.com/octo/repo_two.git",
			configFileContent: configWithoutGeneratedSections,
			want:              []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ini.Load([]byte(tt.configFileContent))
			if err != nil {
				t.Fatal(err)
			}

			gotSections := getGeneratedSectionByRegistryURL(f, tt.repoURL)

			var got []string
			for _, sec := range gotSections {
				got = append(got, sec.Name())
			}

			if len(tt.want) != len(got) {
				t.Errorf("Got %v Want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateNewGrantedSection(t *testing.T) {
	type Registry struct {
		fileContent string
		repoURL     string
	}

	tests := []struct {
		name              string
		want              []string
		registries        []Registry
		configFileContent string
	}{
		{
			name: "two registry with duplicate profiles",
			registries: []Registry{
				{
					fileContent: profileRegistryOne,
					repoURL:     "https://github.com/octo/repo_one.git",
				}, {
					fileContent: profileRegistryTwo,
					repoURL:     "https://github.com/octo/repo_two.git",
				}},
			configFileContent: "",
			want:              []string{"DEFAULT", "granted_registry_start https://github.com/octo/repo_one.git", "profile s1.one", "profile s1.two", "profile duplicate", "granted_registry_end https://github.com/octo/repo_one.git", "granted_registry_start https://github.com/octo/repo_two.git", "profile s2.one", "profile s2.two", "profile repo_two.duplicate", "granted_registry_end https://github.com/octo/repo_two.git"},
		},
		{
			// this is now possible because user can `granted registry add abc.git/custom1.yml` & `granted registry add abc.git/custom2.yml`
			// custom1.yml & custom2.yml might share same aws config file.
			name: "same registry added from multiple config.yml file",
			registries: []Registry{
				{
					fileContent: profileRegistryOne,
					repoURL:     "https://github.com/octo/repo_one.git",
				}, {
					fileContent: profileRegistryOne,
					repoURL:     "https://github.com/octo/repo_one.git/config.yml",
				}},
			configFileContent: configWithoutGeneratedSections,
			want:              []string{"DEFAULT", "granted_registry_start https://github.com/octo/repo_one.git", "profile s1.one", "profile s1.two", "profile duplicate", "granted_registry_end https://github.com/octo/repo_one.git", "granted_registry_start https://github.com/octo/repo_one.git/config.yml", "profile repo_one_config.yml.s1.one", "profile repo_one_config.yml.s1.two", "profile repo_one_config.yml.duplicate", "granted_registry_end https://github.com/octo/repo_one.git/config.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFileContent, err := ini.Load([]byte(""))
			if err != nil {
				t.Fatal(err)
			}

			for _, r := range tt.registries {
				clonedFile, err := ini.Load([]byte(r.fileContent))
				if err != nil {
					t.Fatal(err)
				}

				err = generateNewRegistrySection(configFileContent, clonedFile, r.repoURL, false)
				if err != nil {
					t.Fatal(err)
				}

				if err = configFileContent.Reload(); err != nil {
					t.Fatal(err)
				}
			}

			got := configFileContent.SectionStrings()

			isEqual, invalidArr := CheckArrayEquality(tt.want, got)

			if !isEqual {
				if len(invalidArr) > 0 {
					t.Errorf("invalid values %v on want %v", invalidArr, tt.want)

					return
				}

				t.Errorf("Unequal array length. want %v and got %v", tt.want, got)
			}
		})
	}
}

func CheckArrayEquality(want []string, got []string) (bool, []string) {
	if len(want) != len(got) {
		return false, []string{}
	}

	var inValidValues []string = []string{}
	for _, v := range got {
		var isValid bool = false
		for _, k := range want {
			if k == v {
				isValid = true
				continue
			}
		}

		if !isValid {
			inValidValues = append(inValidValues, v)
		}
	}

	if len(inValidValues) > 0 {
		return false, inValidValues
	}

	return true, []string{}
}

const ConfigWithGeneratedSections = `#
[profile before.1]
key=value
key1=value2

[profile before.2]
key2=value2
key3=value3

### GRANTED-REGISTRY-SECTION: "https://github.com/Eddie023/aws-config-sync-test.git". DO NOT EDIT.
# This section is automatically generated by Granted (https://granted.dev). Manual edits to this section will be overwritten.
# To edit, clone "https://github.com/Eddie023/aws-config-sync-test.git", edit granted.yml, and push your changes. You may need to make a pull request depending on the repository settings.
# To stop syncing and remove this section, run 'granted registry remove https://github.com/Eddie023/aws-config-sync-test.git

[granted_registry_start https://github.com/octo/repo_one.git]

# random comment
[profile s1.one]
region = us-east-2

[profile s1.two]
granted_sso_start_url  = https://example.awsapps.com/start

[profile duplicate]
region = us-east-1

[granted_registry_end https://github.com/octo/repo_one.git]

[granted_registry_start https://github.com/octo/repo_two.git]

[profile duplicate]
region = ap-south-east-2 

[profile s2.one]
granted_sso_start_url  = https://example.awsapps.com/start
credential_process     = granted credential-process --profile dev

[profile s2.two]
granted_sso_start_url  = https://example.awsapps.com/start
credential_process     = granted credential-process --profile dev

[profile s3.three]
credential_process     = granted credential-process --profile dev

[granted_registry_end https://github.com/octo/repo_two.git]

[profile after.1]
a = b 

[profile after.2]
x = z`

const configWithoutGeneratedSections = `
[profile one]
key=value
key1=value2

[profile two]
key2=value2
key3=value3

#some comments

[profile three]
key3=value3
key4=value4

## random things
`

const profileRegistryOne = `
[profile s1.one]
region = us-east-2

[profile s1.two]
granted_sso_start_url  = https://example.awsapps.com/start

[profile duplicate]
region = us-east-1
`

const profileRegistryTwo = `
[profile s2.one]
region = us-east-2

[profile s2.two]
granted_sso_start_url  = https://example.awsapps.com/start

[profile duplicate]
region = ap-southeast-2
`
