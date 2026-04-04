## Integration Points

Call `activity.Record()` from these locations after merge:

- `issue.Create` -> `Record(repoID, authorID, "issue_opened", "issue", issueID, &number, {"title": title})`
- `issue.Update` (status->closed) -> `Record(repoID, authorID, "issue_closed", "issue", issueID, &number, {"title": title})`
- `pull.Create` -> `Record(repoID, authorID, "pr_opened", "pr", prID, &number, {"title": title})`
- `pull.Merge` -> `Record(repoID, mergerID, "pr_merged", "pr", prID, &number, {"title": title})`
- `pull.Update` (status->closed) -> `Record(repoID, authorID, "pr_closed", "pr", prID, &number, {"title": title})`
- `review.Create` -> `Record(repoID, authorID, "review_submitted", "review", reviewID, &prNumber, {"type": type, "pr_title": title})`
- `comment.Create` -> `Record(repoID, authorID, "comment_created", "comment", commentID, nil, {"body_preview": first100chars})`
- `repo.Create` -> `Record(&repoID, ownerID, "repo_created", "repo", &repoID, nil, {"name": name})`

### Event Types

| event_type | ref_type | ref_id | ref_number | payload keys |
|---|---|---|---|---|
| `issue_opened` | `issue` | issue UUID | issue number | `title` |
| `issue_closed` | `issue` | issue UUID | issue number | `title` |
| `pr_opened` | `pr` | PR UUID | PR number | `title` |
| `pr_merged` | `pr` | PR UUID | PR number | `title` |
| `pr_closed` | `pr` | PR UUID | PR number | `title` |
| `review_submitted` | `review` | review UUID | PR number | `type`, `pr_title` |
| `comment_created` | `comment` | comment UUID | - | `body_preview` |
| `repo_created` | `repo` | repo UUID | - | `name` |
| `push` | `commit` | - | - | `branch`, `commits`, `count` |
