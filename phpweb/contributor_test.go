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

package phpweb

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudfoundry/libcfbuildpack/buildpackplan"

	"github.com/cloudfoundry/php-web-cnb/procmgr"

	"github.com/cloudfoundry/libcfbuildpack/helper"
	"github.com/cloudfoundry/libcfbuildpack/logger"

	bplogger "github.com/buildpack/libbuildpack/logger"
	"github.com/cloudfoundry/libcfbuildpack/layers"
	"github.com/cloudfoundry/libcfbuildpack/test"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestUnitContributor(t *testing.T) {
	spec.Run(t, "Contributor", testContributor, spec.Report(report.Terminal{}))
}

func testContributor(t *testing.T, when spec.G, it spec.S) {
	var f *test.BuildFactory
	var c Contributor

	it.Before(func() {
		RegisterTestingT(t)
		f = test.NewBuildFactory(t)

		f.AddPlan(buildpackplan.Plan{Name: Dependency})

		var err error
		c, _, err = NewContributor(f.Build)
		Expect(err).To(Not(HaveOccurred()))

		Expect(helper.WriteFile(filepath.Join(f.Build.Buildpack.Root, "bin", "procmgr"), os.ModePerm, "")).To(Succeed())
	})

	when("creating a new contributor", func() {
		it("generates random Metadata to prevent php-web layer from being cached", func() {
			c, _, err := NewContributor(f.Build)
			Expect(err).To(Not(HaveOccurred()))

			Expect(c.metadata.Name).To(Equal("PHP Web"))
			Expect(len(c.metadata.Hash)).To(Equal(64))
		})
	})

	when("starting a web app", func() {
		it.Before(func() {
			buildDir := filepath.Join(f.Build.Application.Root, "htdocs", "index.php")
			helper.WriteFile(buildDir, 0644, "junk")
		})

		it("starts a web app with `php -S`", func() {
			c.buildpackYAML.Config.WebServer = PhpWebServer

			Expect(c.Contribute()).To(Succeed())

			command := fmt.Sprintf("php -S 0.0.0.0:$PORT -t %s/%s", f.Build.Application.Root, "htdocs")
			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "task", Command: command},
					{Type: "web", Command: command},
				},
			}))
		})

		it("starts a web app with HTTPD", func() {
			c.buildpackYAML.Config.WebServer = ApacheHttpd

			Expect(c.Contribute()).To(Succeed())

			phpLayer := f.Build.Layers.Layer(Dependency)
			procFile := filepath.Join(phpLayer.Root, "procs.yml")

			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "web", Command: fmt.Sprintf("procmgr %s", procFile)},
				},
			}))

			Expect(procFile).To(BeARegularFile())
			procs, err := procmgr.ReadProcs(procFile)
			Expect(err).ToNot(HaveOccurred())

			phpFpmProc := procmgr.Proc{
				Command: "php-fpm",
				Args:    []string{"-p", phpLayer.Root, "-y", filepath.Join(phpLayer.Root, "etc", "php-fpm.conf"), "-c", filepath.Join(phpLayer.Root, "etc")},
			}

			httpdProc := procmgr.Proc{
				Command: "httpd",
				Args:    []string{"-f", filepath.Join(c.application.Root, "httpd.conf"), "-k", "start", "-DFOREGROUND"},
			}

			Expect(procs.Processes).To(ContainElement(phpFpmProc))
			Expect(procs.Processes).To(ContainElement(httpdProc))
		})

		it("starts a web app and defaults to Apache Web Server", func() {
			Expect(c.Contribute()).To(Succeed())

			phpLayer := f.Build.Layers.Layer(Dependency)
			procFile := filepath.Join(phpLayer.Root, "procs.yml")

			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "web", Command: fmt.Sprintf("procmgr %s", procFile)},
				},
			}))

			Expect(procFile).To(BeARegularFile())

			file, err := os.Open(procFile)
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()

			buf, err := ioutil.ReadAll(file)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(buf)).To(ContainSubstring("command: php-fpm"))
			Expect(string(buf)).To(ContainSubstring("-p"))
			Expect(string(buf)).To(ContainSubstring("layers/php-web"))
			Expect(string(buf)).To(ContainSubstring("-y"))
			Expect(string(buf)).To(ContainSubstring("layers/php-web/etc/php-fpm.conf"))
			Expect(string(buf)).To(ContainSubstring("-c"))
			Expect(string(buf)).To(ContainSubstring("layers/php-web/etc"))

			Expect(string(buf)).To(ContainSubstring("command: httpd"))
			Expect(string(buf)).To(ContainSubstring("-f"))
			Expect(string(buf)).To(ContainSubstring("application"))
			Expect(string(buf)).To(ContainSubstring("-k"))
			Expect(string(buf)).To(ContainSubstring("start"))
			Expect(string(buf)).To(ContainSubstring("-DFOREGROUND"))
		})

		it("starts a web app with a custom webdir", func() {
			c.buildpackYAML.Config.WebServer = PhpWebServer
			c.buildpackYAML.Config.WebDirectory = "public"

			buildDir := filepath.Join(f.Build.Application.Root, c.buildpackYAML.Config.WebDirectory, "index.php")
			helper.WriteFile(buildDir, 0644, "junk")

			Expect(c.Contribute()).To(Succeed())

			command := fmt.Sprintf("php -S 0.0.0.0:$PORT -t %s/%s", f.Build.Application.Root, "public")
			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "task", Command: command},
					{Type: "web", Command: command},
				},
			}))
		})

		it("starts a web app with NGINX", func() {
			c.buildpackYAML.Config.WebServer = Nginx

			Expect(c.Contribute()).To(Succeed())

			phpLayer := f.Build.Layers.Layer(Dependency)
			procFile := filepath.Join(phpLayer.Root, "procs.yml")

			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "web", Command: fmt.Sprintf("procmgr %s", procFile)},
				},
			}))

			Expect(procFile).To(BeARegularFile())
			procs, err := procmgr.ReadProcs(procFile)
			Expect(err).ToNot(HaveOccurred())

			phpFpmProc := procmgr.Proc{
				Command: "php-fpm",
				Args:    []string{"-p", phpLayer.Root, "-y", filepath.Join(phpLayer.Root, "etc", "php-fpm.conf"), "-c", filepath.Join(phpLayer.Root, "etc")},
			}

			nginxProc := procmgr.Proc{
				Command: "nginx",
				Args:    []string{"-p", c.application.Root, "-c", filepath.Join(c.application.Root, "nginx.conf")},
			}

			Expect(procs.Processes).To(ContainElement(phpFpmProc))
			Expect(procs.Processes).To(ContainElement(nginxProc))
		})

		it("contributes php-fpm.conf & includes a user's config", func() {
			helper.WriteFile(filepath.Join(f.Build.Application.Root, ".php.fpm.d", "user.conf"), 0644, "")

			layer := f.Build.Layers.Layer(Dependency)
			err := c.writePhpFpmConf(layer, "")

			Expect(err).ToNot(HaveOccurred())
			Expect(filepath.Join(layer.Root, "etc", "php-fpm.conf")).To(BeARegularFile())

			result, err := ioutil.ReadFile(filepath.Join(layer.Root, "etc", "php-fpm.conf"))

			Expect(err).ToNot(HaveOccurred())
			Expect(string(result)).To(ContainSubstring(fmt.Sprintf(`include=%s`, filepath.Join(f.Build.Application.Root, ".php.fpm.d", "*.conf"))))
		})

		when("the requested web server is not supported", func() {
			it("does not provide a start command", func() {
				c.buildpackYAML.Config.WebServer = "notsupportedserver"

				Expect(c.Contribute()).To(Succeed())

				phpLayer := f.Build.Layers.Layer(Dependency)
				procFile := filepath.Join(phpLayer.Root, "procs.yml")
				Expect(procFile).ToNot(BeAnExistingFile())
			})
		})
	})

	when("contributing to build", func() {
		when("it's a web app", func() {
			it.Before(func() {
				buildDir := filepath.Join(f.Build.Application.Root, "htdocs", "index.php")
				helper.WriteFile(buildDir, 0644, "junk")
			})

			it("contributes a php.ini file & configures PHP to look at it for a web app", func() {
				c.buildpackYAML.Config.WebServer = PhpWebServer

				layer := f.Build.Layers.Layer(Dependency)
				Expect(c.Contribute()).To(Succeed())
				Expect(filepath.Join(layer.Root, "etc", "php.ini")).To(BeARegularFile())
				Expect(layer).To(test.HaveOverrideSharedEnvironment("PHPRC", filepath.Join(layer.Root, "etc")))
				Expect(layer).To(test.HaveOverrideSharedEnvironment("PHP_INI_SCAN_DIR", filepath.Join(f.Build.Application.Root, ".php.ini.d")))
			})

			it("contributes a httpd.conf & php-fpm.conf file when using Apache Web Server", func() {
				c.buildpackYAML.Config.WebServer = ApacheHttpd

				layer := f.Build.Layers.Layer(Dependency)
				Expect(c.Contribute()).To(Succeed())
				Expect(filepath.Join(f.Build.Application.Root, "httpd.conf")).To(BeARegularFile())
				Expect(filepath.Join(layer.Root, "etc", "php-fpm.conf")).To(BeARegularFile())
			})

			it("contributes a nginx.conf & php-fpm.conf file when using Nginx", func() {
				c.buildpackYAML.Config.WebServer = Nginx

				layer := f.Build.Layers.Layer(Dependency)
				Expect(c.Contribute()).To(Succeed())
				Expect(filepath.Join(f.Build.Application.Root, "nginx.conf")).To(BeARegularFile())
				Expect(filepath.Join(layer.Root, "etc", "php-fpm.conf")).To(BeARegularFile())
			})
		})

		when("it's not a web app", func() {
			it("contributes a php.ini file & configures PHP to look at it for a script", func() {
				layer := f.Build.Layers.Layer(Dependency)
				Expect(c.Contribute()).To(Succeed())
				Expect(filepath.Join(layer.Root, "etc", "php.ini")).To(BeARegularFile())
				Expect(layer).To(test.HaveOverrideSharedEnvironment("PHPRC", filepath.Join(layer.Root, "etc")))
				Expect(layer).To(test.HaveOverrideSharedEnvironment("PHP_INI_SCAN_DIR", filepath.Join(f.Build.Application.Root, ".php.ini.d")))
			})
		})
	})

	when("starting a PHP script", func() {
		it("starts a script using default `app.php`", func() {

			for _, script := range DefaultCliScripts {
				scriptName := filepath.Join(f.Build.Application.Root, script)
				err := helper.WriteFile(scriptName, 0655, "")
				Expect(err).ToNot(HaveOccurred())

				Expect(c.contributeScript(f.Build.Layers.Layer(fmt.Sprintf("layer-%s", script)))).To(Succeed())

				command := fmt.Sprintf("php %s/%s", f.Build.Application.Root, script)
				Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
					Processes: []layers.Process{
						{Type: "task", Command: command},
						{Type: "web", Command: command},
					},
				}))

				os.Remove(scriptName)
			}
		})

		it("starts a script using custom script path/name", func() {
			c.buildpackYAML.Config.Script = "relative/path/to/my/script.php"

			Expect(c.Contribute()).To(Succeed())

			command := fmt.Sprintf("php %s/%s", f.Build.Application.Root, "relative/path/to/my/script.php")
			Expect(f.Build.Layers).To(test.HaveApplicationMetadata(layers.Metadata{
				Processes: []layers.Process{
					{Type: "task", Command: command},
					{Type: "web", Command: command},
				},
			}))
		})

		it("logs a warning when start script does not exist", func() {
			debug := &bytes.Buffer{}
			info := &bytes.Buffer{}

			c.logger = logger.Logger{Logger: bplogger.NewLogger(debug, info)}

			Expect(c.Contribute()).To(Succeed())
			Expect(info.String()).To(ContainSubstring("Buildpack could not find a file to execute. Either set php.script in buildpack.yml or include one of these files [app.php, main.php, run.php, start.php]"))
		})
	})

}
