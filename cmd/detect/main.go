/*
 * Copyright 2018-2019 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/php-web-cnb/phpweb"

	"github.com/buildpack/libbuildpack/buildplan"
	"github.com/cloudfoundry/libcfbuildpack/detect"
	"github.com/cloudfoundry/libcfbuildpack/logger"
	"github.com/cloudfoundry/php-cnb/php"
)

func main() {
	detectionContext, err := detect.DefaultDetect()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to run detection: %s", err)
		os.Exit(101)
	}

	code, err := runDetect(detectionContext)
	if err != nil {
		detectionContext.Logger.Info(err.Error())
	}

	os.Exit(code)
}

func pickWebDir(buildpackYAML php.BuildpackYAML) string {
	if buildpackYAML.Config.WebDirectory != "" {
		return buildpackYAML.Config.WebDirectory
	}

	return "htdocs"
}

func searchForWebApp(appRoot string, webdir string) (bool, error) {
	matchList, err := filepath.Glob(filepath.Join(appRoot, webdir, "*.php"))
	if err != nil {
		return false, err
	}

	if len(matchList) > 0 {
		return true, nil
	}
	return false, nil
}

func searchForScript(appRoot string, log logger.Logger) (bool, error) {
	found := false

	err := filepath.Walk(appRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Info("failure accessing a path %q: %v\n", path, err)
			return filepath.SkipDir
		}

		if found {
			return filepath.SkipDir
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".php") {
			found = true
		}

		return nil
	})

	if err != nil {
		return false, err
	}

	return found, nil
}

func runDetect(context detect.Detect) (int, error) {
	buildpackYAML, err := php.LoadBuildpackYAML(context.Application.Root)
	if err != nil {
		return context.Fail(), err
	}

	webdir := pickWebDir(buildpackYAML)

	webAppFound, err := searchForWebApp(context.Application.Root, webdir)
	if err != nil {
		return context.Fail(), err
	}

	if webAppFound {
		return context.Pass(buildplan.BuildPlan{
			php.Dependency: buildplan.Dependency{
				Metadata: buildplan.Metadata{
					"launch": true,
				},
			},
			phpweb.WebDependency: buildplan.Dependency{},
		})
	}

	scriptFound, err := searchForScript(context.Application.Root, context.Logger)
	if err != nil {
		return context.Fail(), err
	}

	if scriptFound {
		return context.Pass(buildplan.BuildPlan{
			php.Dependency: buildplan.Dependency{
				Metadata: buildplan.Metadata{
					"launch": true,
				},
			},
			phpweb.ScriptDependency: buildplan.Dependency{},
		})
	}

	return context.Fail(), nil
}
