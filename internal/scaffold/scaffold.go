package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"go.digitalxero.dev/stdb-go/templates"
)

// ProjectType represents the type of project to scaffold.
type ProjectType string

const (
	// Server scaffolds a SpacetimeDB server module.
	Server ProjectType = "server"
	// Client scaffolds a SpacetimeDB Go client.
	Client ProjectType = "client"
	// Fullstack scaffolds both server and client with an orchestrating Taskfile.
	Fullstack ProjectType = "fullstack"
)

// ScaffoldBuilder configures and builds a Scaffold.
type ScaffoldBuilder interface {
	WithName(name string) ScaffoldBuilder
	WithModule(module string) ScaffoldBuilder
	WithType(t ProjectType) ScaffoldBuilder
	WithDir(dir string) ScaffoldBuilder
	WithClientSDKVersion(v string) ScaffoldBuilder
	WithServerSDKVersion(v string) ScaffoldBuilder
	Build() (Scaffold, error)
}

// Scaffold generates project files.
type Scaffold interface {
	Generate() error
}

type templateData struct {
	Name             string
	Module           string
	TableName        string
	TypeName         string
	ClientSDKVersion string
	ServerSDKVersion string
}

type scaffold struct {
	name             string
	module           string
	projectType      ProjectType
	dir              string
	clientSDKVersion string
	serverSDKVersion string
	data             templateData
}

// NewScaffoldBuilder creates a new ScaffoldBuilder with defaults.
func NewScaffoldBuilder() ScaffoldBuilder {
	return &scaffold{
		projectType: Server,
	}
}

func (s *scaffold) WithName(name string) ScaffoldBuilder {
	s.name = name
	return s
}

func (s *scaffold) WithModule(module string) ScaffoldBuilder {
	s.module = module
	return s
}

func (s *scaffold) WithType(t ProjectType) ScaffoldBuilder {
	s.projectType = t
	return s
}

func (s *scaffold) WithDir(dir string) ScaffoldBuilder {
	s.dir = dir
	return s
}

func (s *scaffold) WithClientSDKVersion(v string) ScaffoldBuilder {
	s.clientSDKVersion = v
	return s
}

func (s *scaffold) WithServerSDKVersion(v string) ScaffoldBuilder {
	s.serverSDKVersion = v
	return s
}

func (s *scaffold) Build() (Scaffold, error) {
	if s.name == "" {
		return nil, fmt.Errorf("scaffold: project name is required")
	}

	if s.module == "" {
		s.module = s.name
	}

	if s.dir == "" {
		s.dir = filepath.Join(".", s.name)
	}

	switch s.projectType {
	case Server, Client, Fullstack:
		// valid
	default:
		return nil, fmt.Errorf("scaffold: invalid project type %q (must be server, client, or fullstack)", s.projectType)
	}

	if s.clientSDKVersion == "" {
		s.clientSDKVersion = latestModuleVersion(clientSDKModule, fallbackClientSDKVersion)
	}

	if s.serverSDKVersion == "" {
		s.serverSDKVersion = latestModuleVersion(serverSDKModule, fallbackServerSDKVersion)
	}

	s.data = templateData{
		Name:             s.name,
		Module:           s.module,
		TableName:        "user",
		TypeName:         "User",
		ClientSDKVersion: s.clientSDKVersion,
		ServerSDKVersion: s.serverSDKVersion,
	}

	return s, nil
}

func (s *scaffold) Generate() error {
	switch s.projectType {
	case Server:
		return s.generateFromDir("server", s.dir)
	case Client:
		return s.generateFromDir("client", s.dir)
	case Fullstack:
		if err := s.generateFromDir("server", filepath.Join(s.dir, "server")); err != nil {
			return err
		}
		clientData := s.data
		clientData.Module = s.module + "/client"
		origData := s.data
		s.data = clientData
		if err := s.generateFromDir("client", filepath.Join(s.dir, "client")); err != nil {
			s.data = origData
			return err
		}
		s.data = origData
		return s.generateFromDir("fullstack", s.dir)
	default:
		return fmt.Errorf("scaffold: unsupported project type %q", s.projectType)
	}
}

func (s *scaffold) generateFromDir(tmplDir, outputDir string) error {
	entries, err := templates.FS.ReadDir(tmplDir)
	if err != nil {
		return fmt.Errorf("scaffold: reading templates/%s: %w", tmplDir, err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("scaffold: creating directory %s: %w", outputDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		tmplPath := tmplDir + "/" + entry.Name()
		content, err := templates.FS.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("scaffold: reading template %s: %w", tmplPath, err)
		}

		tmpl, err := template.New(entry.Name()).Parse(string(content))
		if err != nil {
			return fmt.Errorf("scaffold: parsing template %s: %w", tmplPath, err)
		}

		// Strip .tmpl extension for output filename
		outputName := entry.Name()
		if ext := filepath.Ext(outputName); ext == ".tmpl" {
			outputName = outputName[:len(outputName)-len(ext)]
		}

		outputPath := filepath.Join(outputDir, outputName)

		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("scaffold: creating %s: %w", outputPath, err)
		}

		if err := tmpl.Execute(f, s.data); err != nil {
			_ = f.Close()
			return fmt.Errorf("scaffold: executing template %s: %w", tmplPath, err)
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("scaffold: closing %s: %w", outputPath, err)
		}
	}

	return nil
}
