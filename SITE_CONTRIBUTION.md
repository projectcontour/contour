# Website Contributions

[projectcontour.io][9] is a static HTML site built using the Hugo static site generator. All site content is written in Markdown.

Contribution [guidelines][4] and [workflows][3] still apply to site contributions.

Please familiarize yourself with Hugo and Markdown if you intend on contributing to the site.

- [Hugo documentation][1]
- [Mastering Markdown][2]

## Prerequisites
- You must have Hugo installed. Follow the [Hugo installation][10] instructions for your platform. To verify the installation, you should see a list of help commands when you run the following command:
    ```
    hugo help
    ```

- A local copy of the contour repository

## Understanding the directory structure
The website content is located in the `site/content` directory. 
The `site` directory is organized as follows:
```
site
├───archetypes
├───content
│   ├───community # Community page
│   ├───docs # Versioned product documentation
│   │   ├───main
│   │   ├───v1.0.0
│   │   ├───v1.0.1
│   │   ├─── ...
│   ├───examples
│   ├───getting-started # Getting started guide
│   ├───guides # Guides to configuring specific features
│   ├───posts # Blog posts
│   └───resources # Resources such as videos, podcasts, and community articles
├───data
├───img
├───public # Blog
├───resources
└───themes
```


## Linking to resources
Reference table links are preferred over inline links as reference tables make it easier to find and update links.
Reference table links use the following Markdown format:
```
If you encounter issues, review the [troubleshooting][17] page, [file an issue][6], or talk to us on the [#contour channel][12] on Kubernetes Slack.
```

A reference table, located at the end of the Markdown file, uses the following format:
```
[1]: /docs/{{< param latest_version >}}/deploy-options
[2]: /CONTRIBUTING.md#contribution-workflow
[3]: {{< param github_url >}}/issues
[4]: https://httpbin.org/
[5]: https://github.com/projectcontour/community/wiki/Office-Hours
[6]: {{< param slack_url >}}
[7]: https://github.com/bitnami/charts/tree/master/bitnami/contour
[8]: https://www.youtube.com/watch?v=xUJbTnN3Dmw
```

## Using URL parameters
Several URL parameters are available for you to use when creating reference table links:

- base_url: "https://projectcontour.io"
- twitter_url: "https://twitter.com/projectcontour"
- github_url: "https://github.com/projectcontour/contour"
- slack_url: "https://kubernetes.slack.com/messages/contour"
- latest_version: latest document version

You can use parameters to build URL strings in the link reference table. For example:
```
[1]: {{< param github_url >}}/issues
[2]: /docs/{{< param latest_version >}}/config/fundamentals
[3]: {{< param github_url>}}/tree/{{< param version >}}/examples/contour
[4]: {{< param slack_url >}}
[5]: {{< param base_url >}}/resources/ philosophy
```


## Using notices
Notices are used to call out specific information. There are four types of notices:
- Tip
- Information
- Warning
- Notes

Notice types use the following format:
```
{{< notice tip >}}
This is a TIP
{{< /notice >}}

{{< notice info >}}
This is INFORMATION
{{< /notice >}}

{{< notice warning >}}
This is a WARNING
{{< /notice >}}

{{< notice note >}}
This is a NOTE
{{< /notice >}}
```

## Proofreading and spelling
Most IDEs have spellcheck or a spellcheck plugin to assist with identifying common spelling errors. If you would like, have someone else review your changes as an additional check.

## Testing your changes
To test your website changes, run the following command from the `site` directory:
```
hugo server
```

Go to `http://localhost:1313` to view the website and changes in your browser. When ready, commit and push your changes to GitHub and create a Pull Request (PR). You can let the team know in the [Contour Slack channel][7] that you have created a PR.


## Next steps
Have questions? Send a Slack message on the Contour channel, an email on the mailing list, or join a Contour meeting.
- Slack: kubernetes.slack.com [#contour][7]
- Join us in a [User Group][5] or [Office Hours][6] meeting 
- Join the [mailing list][8] for the latest information



[1]: https://gohugo.io/documentation/
[2]: https://guides.github.com/features/mastering-markdown/
[3]: /CONTRIBUTING.md#contribution-workflow
[4]: /CONTRIBUTING.md
[5]: https://projectcontour.io/community/
[6]: https://github.com/projectcontour/community/wiki/Office-Hours
[7]: https://kubernetes.slack.com/messages/contour
[8]: https://lists.cncf.io/g/cncf-contour-users/
[9]: https://projectcontour.io
[10]: https://gohugo.io/getting-started/installing/