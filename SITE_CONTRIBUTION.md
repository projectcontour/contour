# Website Contributions

The projectcontour.io site is a static HTML site built using the Jekyll platform. Site content is written in markdown with liquid templating to populate content sections such as links.

Contribution [guidelines][8] and [workflows][7] still apply to site contributions.

Please familiarize yourself with Jekyll and Liquid templating if you intend on contributing to the site.

- [Jekyll documentation][1]
- [Liquid Templating][2]
- [Mastering Markdown][5]

## Directory Structure

Overview of the site directory structure and purpose of directories:

```
site
├── _contributors
├── css # Fonts
│   └── fonts
│       ├── IcoMoon
│       └── Metropolis
├── _data # Custom data files
├── docs # Versioned product documentation
│   ├── master
│   └── v1.0.0
├── _guides # Unversioned product usage guides
├── img
│   ├── case-study-icons
│   ├── cert-manager
│   ├── contour-1.0
│   ├── contributors
│   ├── heroes
│   └── posts
├── _includes # HTML partials that are incorporated into layouts
├── js # Javascript files
├── _layouts # Layout templates to wrap posts and pages
├── _metrics # Content is auto generated. Do not modify contents
├── _plugins # Ruby files for custom Jekyll plugins
├── _posts # Contour blog posts
├── _resources # General information pages
└── _scss # Site style sheets
    ├── bootstrap-4.1.3
    │   ├── mixins
    │   └── utilities
    └── site
        ├── common
        ├── layouts
        ├── objects
        ├── settings
        └── utilities
```

## Link Insertion Standards

Usage of links within a page has been standardized to simplify the process of finding and fixing links as required.

Avoid using inline links, instead, use a reference table the bottom of the file; making it easier to find and update links. [Reference Links][3]. There are some exceptions to this rule, such as deprecation warnings.

There are several conditions which will determine to correct type of link templating to use.

**Linking to a blog post:**

Use this formation when linking to a projectcontour.io blog post. `{% post_url 2010-07-21-name-of-post %}`

This method will follow any site / page changes to permalinks and Jekyll will validate the link during build.

**Linking to a projectcontour.io page:**

This template is used when linking to another projectcontour.io page. Path to the .md is relative from /site/.
`{% link getting-started.md %}` `{% link docs/v1.0.0/deploy-options.md %}`

Jekyll will perform validation against links inserted with this method.

**Linking to a section within the same page:**

Links are created for all headers when the site is built. You can link to sections within the same page by using `#section-text`.
Spaces within a heading are replaced with a hyphen. `## My heading` has a link `#My-heading`.

**Linking to a section within another page:**

Unfortunately, the `{% link %}` templating method does not allow linking to a section within a page. Use this format when linking to a section within another page.
`{% <Path to page post generation/<section> %}` or `/docs/v1.0.0/deploy-options/#Host-Networking`

A key difference between using `{% link %}` and the above method is that `{% link %}` will resolve to pages final location. To use the above method, you need to point the link to the location where the document will end up. For example, files in the `_guides` directory will be located in `/guides` on the live site. Therefore, the link for `_guides/cert-manager` is `/guides/cert-manager`.

Additionally, the above method will not follow any permalink changes or page-specific permalink.

**Linking to the project contour GitHub repo:**

The GitHub metadata plugin populates Jekyll variables with data from GitHub during build. When inserting a link to the Project Contour repository, use the prefix `{{site.github.repository_url}}`.

A full list of variables can be found in the [jekyll-github-metadata documentation][4].

**Linking to an external page:**

Links to external pages are done using the full URL, `[1]: https://projectcontour.io` for example.

## Proofread and spell check

If you're making any content submissions, please proofread and spell check your work.

Spell check can be run using `make check`.

## Local Site Testing

Perform local tests before creating a PR to detect issues such as broken links.

**Using docker:**

The easiest way to run the site locally is by using the top-level [Makefile](../Makefile). This has a `site-devel` target and runs a [Jekyll][6] development server in Docker, which is useful for checking local documentation changes.
By default, the site will be available on [port 4000](http://127.0.0.1:4000/).

Note that the version of [Jekyll][6] used by the site is pinned in the [Gemfile](./Gemfile.lock). The `site-devel` target uses the matching versioned Docker image to ensure compatibility.

Once you are happy with your changes, commit them and push everything
to your fork.  When you submit a PR, Netlify will automatically
generate a preview of your changes.

**Manual Jekyll Instance:**

If you need to manually install and run [Jekyll][6]
without using Docker, you can install the [Jekyll][6]
packages on your development system.

On macOS, install the following:

- `brew install rbenv`
- `rbenv install 2.6.3`
- `gem install bundler`

On Ubuntu you will need these packages:

- ruby
- ruby-dev
- ruby-bundler
- build-essential
- zlib1g-dev
- nginx (or apache2)

Install [Jekyll][6] and plug-ins in one fell
swoop. This mirrors the plug-ins used by GitHub Pages on your local
machine including Jekyll, Sass, etc:

- `gem install github-pages`

Then:

1. Clone down your own fork, or clone the main repo `git clone https://github.com/projectcontour/contour` and add your own remote.
2. `cd site`
3. `rbenv local 2.6.3`
4. `bundle install`
5. Serve the site and watch for markup/sass changes `jekyll serve --livereload`. You may need to run `bundle exec jekyll serve --livereload`.
6. View your website at http://127.0.0.1:4000/

Please ensure that your local version of bundler matches the version specified in `site/Gemfile.lock`. Unintentional changes to the bundler version in `site/Gemfile.lock` can cause issue with the Netlify build process.

[1]: https://jekyllrb.com/docs/
[2]: https://help.shopify.com/en/themes/liquid/basics
[3]: https://sourceforge.net/p/lookup/wiki/markdown_syntax/#md_ex_reflinks
[4]: https://github.com/jekyll/github-metadata/blob/master/docs/site.github.md
[5]: https://guides.github.com/features/mastering-markdown/
[6]: https://jekyllrb.com
[7]: /CONTRIBUTING.md#contribution-workflow
[8]: /CONTRIBUTING.md
