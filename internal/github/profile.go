package github

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"html"
	"image"
	_ "image/gif" // avatars may be GIF, JPEG or PNG
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const profileQuery = `
query GhxProfile {
  viewer {
    login
    name
    bio
    pronouns
    company
    location
    websiteUrl
    twitterUsername
    createdAt
    avatarUrl(size: 96)
    status { emoji message }
    followers { totalCount }
    following { totalCount }
    starredRepositories { totalCount }
    organizations(first: 8) { nodes { login } }
    socialAccounts(first: 4) { nodes { provider displayName url } }
    pinnedItems(first: 6, types: REPOSITORY) {
      nodes { ...ProfileRepo }
    }
    owned: repositories(first: 100, ownerAffiliations: OWNER, isFork: false, orderBy: {field: STARGAZERS, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { ...ProfileRepo }
    }
    allRepos: repositories(ownerAffiliations: OWNER) { totalCount }
    contributionsCollection {
      totalRepositoryContributions
      restrictedContributionsCount
      commitContributionsByRepository(maxRepositories: 6) {
        repository { nameWithOwner }
        contributions { totalCount }
      }
    }
  }
}
` + profileRepoFields

// ownedReposQuery pages through the rest of the viewer's repositories for stars earned and languages.
const ownedReposQuery = `
query GhxOwnedRepos($after: String!) {
  viewer {
    owned: repositories(first: 100, after: $after, ownerAffiliations: OWNER, isFork: false, orderBy: {field: STARGAZERS, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { ...ProfileRepo }
    }
  }
}
` + profileRepoFields

// maxOwnedPages bounds the pagination (1,000 repositories). Pages are sorted by stars, so anything
// beyond it adds almost nothing to the totals.
const maxOwnedPages = 10

const profileRepoFields = `
fragment ProfileRepo on Repository {
  nameWithOwner
  description
  stargazerCount
  forkCount
  isPrivate
  primaryLanguage { name color }
  parent { nameWithOwner }
}`

type gqlProfileRepo struct {
	NameWithOwner   string `json:"nameWithOwner"`
	Description     string `json:"description"`
	StargazerCount  int    `json:"stargazerCount"`
	ForkCount       int    `json:"forkCount"`
	IsPrivate       bool   `json:"isPrivate"`
	PrimaryLanguage *struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"primaryLanguage"`
	Parent *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"parent"`
}

func (r gqlProfileRepo) pinned() PinnedRepo {
	p := PinnedRepo{NameWithOwner: r.NameWithOwner, Description: r.Description, Stars: r.StargazerCount, Forks: r.ForkCount, IsPrivate: r.IsPrivate}
	if r.PrimaryLanguage != nil {
		p.Language, p.LanguageColor = r.PrimaryLanguage.Name, r.PrimaryLanguage.Color
	}
	if r.Parent != nil {
		p.Parent = r.Parent.NameWithOwner
	}
	return p
}

type gqlTotal struct {
	TotalCount int `json:"totalCount"`
}

type gqlRepoPage struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []gqlProfileRepo `json:"nodes"`
}

type gqlProfile struct {
	Viewer struct {
		Login           string    `json:"login"`
		Name            string    `json:"name"`
		Bio             string    `json:"bio"`
		Pronouns        string    `json:"pronouns"`
		Company         string    `json:"company"`
		Location        string    `json:"location"`
		WebsiteURL      string    `json:"websiteUrl"`
		TwitterUsername string    `json:"twitterUsername"`
		CreatedAt       time.Time `json:"createdAt"`
		AvatarURL       string    `json:"avatarUrl"`
		Status          *struct {
			Emoji   string `json:"emoji"`
			Message string `json:"message"`
		} `json:"status"`
		Followers     gqlTotal `json:"followers"`
		Following     gqlTotal `json:"following"`
		Starred       gqlTotal `json:"starredRepositories"`
		Organizations struct {
			Nodes []gqlLogin `json:"nodes"`
		} `json:"organizations"`
		SocialAccounts struct {
			Nodes []struct {
				Provider    string `json:"provider"`
				DisplayName string `json:"displayName"`
				URL         string `json:"url"`
			} `json:"nodes"`
		} `json:"socialAccounts"`
		PinnedItems struct {
			Nodes []gqlProfileRepo `json:"nodes"`
		} `json:"pinnedItems"`
		Owned                   gqlRepoPage `json:"owned"`
		AllRepos                gqlTotal    `json:"allRepos"`
		ContributionsCollection struct {
			TotalRepositoryContributions int `json:"totalRepositoryContributions"`
			RestrictedContributionsCount int `json:"restrictedContributionsCount"`
			CommitContributionsByRepo    []struct {
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				Contributions gqlTotal `json:"contributions"`
			} `json:"commitContributionsByRepository"`
		} `json:"contributionsCollection"`
	} `json:"viewer"`
}

// FetchProfile loads the viewer's profile: details, pinned repositories, the profile README and the
// avatar. The README and avatar are best-effort; missing ones leave their fields empty.
func (c *Client) FetchProfile(ctx context.Context) (*Profile, error) {
	var out gqlProfile
	if err := c.graphQL(ctx, profileQuery, nil, &out); err != nil {
		return nil, err
	}
	v := out.Viewer
	p := &Profile{
		Login: v.Login, Name: v.Name, Bio: strings.TrimSpace(v.Bio), Pronouns: v.Pronouns,
		Company: strings.TrimSpace(v.Company), Location: v.Location, Website: v.WebsiteURL, Twitter: v.TwitterUsername,
		AvatarURL: v.AvatarURL, CreatedAt: v.CreatedAt,
		Followers: v.Followers.TotalCount, Following: v.Following.TotalCount, Starred: v.Starred.TotalCount,
		Repos:        v.AllRepos.TotalCount,
		ReposCreated: v.ContributionsCollection.TotalRepositoryContributions, PrivateContrib: v.ContributionsCollection.RestrictedContributionsCount,
	}
	if v.Status != nil {
		p.StatusEmoji, p.StatusMessage = v.Status.Emoji, strings.TrimSpace(v.Status.Message)
	}
	for _, o := range v.Organizations.Nodes {
		p.Orgs = append(p.Orgs, o.Login)
	}
	for _, s := range v.SocialAccounts.Nodes {
		p.Socials = append(p.Socials, Social{Provider: s.Provider, Name: s.DisplayName, URL: s.URL})
	}
	for _, r := range v.PinnedItems.Nodes {
		if r.NameWithOwner != "" {
			p.Pinned = append(p.Pinned, r.pinned())
		}
	}
	owned := v.Owned.Nodes
	for page, next := 1, v.Owned.PageInfo; next.HasNextPage && page < maxOwnedPages; page++ {
		var more struct {
			Viewer struct {
				Owned gqlRepoPage `json:"owned"`
			} `json:"viewer"`
		}
		if err := c.graphQL(ctx, ownedReposQuery, map[string]any{"after": next.EndCursor}, &more); err != nil {
			break // the first page already covers the most-starred repositories
		}
		owned = append(owned, more.Viewer.Owned.Nodes...)
		next = more.Viewer.Owned.PageInfo
	}
	langCount, langColor, total := map[string]int{}, map[string]string{}, 0
	for _, r := range owned {
		p.StarsEarned += r.StargazerCount
		p.ForksEarned += r.ForkCount
		if r.PrimaryLanguage != nil {
			langCount[r.PrimaryLanguage.Name]++
			langColor[r.PrimaryLanguage.Name] = r.PrimaryLanguage.Color
			total++
		}
	}
	p.Languages = buildLanguages(langCount, langColor, total)
	// Like github.com: without pins, show the most-starred public repositories instead.
	if len(p.Pinned) == 0 {
		p.Popular = true
		for _, r := range owned {
			if !r.IsPrivate && len(p.Pinned) < 6 {
				p.Pinned = append(p.Pinned, r.pinned())
			}
		}
	}
	for _, r := range v.ContributionsCollection.CommitContributionsByRepo {
		p.CommitRepos = append(p.CommitRepos, RepoContribution{Repo: r.Repository.NameWithOwner, Commits: r.Contributions.TotalCount})
	}
	sort.SliceStable(p.CommitRepos, func(i, j int) bool { return p.CommitRepos[i].Commits > p.CommitRepos[j].Commits })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.Readme, _ = c.FetchProfileReadme(ctx, p.Login)
	}()
	go func() {
		defer wg.Done()
		p.Avatar, _ = c.FetchAvatar(ctx, p.AvatarURL, 32)
	}()
	wg.Wait()
	return p, nil
}

// FetchProfileReadme returns the profile README (the <login>/<login> repository) as plain text,
// or "" when the viewer has none.
func (c *Client) FetchProfileReadme(ctx context.Context, login string) (string, error) {
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/%s/readme", login, login), &out); err != nil {
		if IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	raw := []byte(out.Content)
	if out.Encoding == "base64" {
		b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
		if err != nil {
			return "", err
		}
		raw = b
	}
	return CleanReadme(string(raw)), nil
}

// FetchAvatar downloads an avatar image and shrinks it to size×size pixels. The request carries no
// credentials: avatars are public and live on another host than the API.
func (c *Client) FetchAvatar(ctx context.Context, url string, size int) (*Pixels, error) {
	if url == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar: HTTP %d", resp.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("avatar: %w", err)
	}
	return Downscale(img, size), nil
}

// Downscale box-filters an image to a size×size square, cropping the longer side to center it.
// Transparent areas are composited onto white, as github.com shows them.
func Downscale(img image.Image, size int) *Pixels {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	px := &Pixels{W: size, H: size, RGB: make([]byte, 0, size*size*3)}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx0, sx1 := x0+x*side/size, x0+max((x+1)*side/size, x*side/size+1)
			sy0, sy1 := y0+y*side/size, y0+max((y+1)*side/size, y*side/size+1)
			var r, g, bl, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					white := 0xffff - uint64(ca)
					r += uint64(cr) + white
					g += uint64(cg) + white
					bl += uint64(cb) + white
					n++
				}
			}
			px.RGB = append(px.RGB, byte(r/n>>8), byte(g/n>>8), byte(bl/n>>8))
		}
	}
	return px
}

var (
	mdComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	mdHTMLHead  = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	mdHTMLBreak = regexp.MustCompile(`(?i)<br\s*/?>|</?(p|div|ul|ol|table|tr|details|summary)\b[^>]*>`)
	mdHTMLItem  = regexp.MustCompile(`(?i)<li[^>]*>`)
	mdHTMLTag   = regexp.MustCompile(`<[^>]+>`)
	mdImage     = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)|!\[[^\]]*\]\[[^\]]*\]`)
	mdLink      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)|\[([^\]]*)\]\[[^\]]*\]`)
	mdRefDef    = regexp.MustCompile(`^\s*\[[^\]]+\]:\s*\S+`)
	mdHeading   = regexp.MustCompile(`^#{1,6}\s+`)
	mdBullet    = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+`)
	mdEmphasis  = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~`)
	mdTableRule = regexp.MustCompile(`^\s*\|?\s*:?-{3,}`)
	mdRule      = regexp.MustCompile(`^\s*([-*_]\s*){3,}$`)
)

// CleanReadme turns a profile README into plain text a terminal can show. Profile READMEs are mostly
// HTML and badge images, so images, comments and tags are dropped, links keep their text, headings
// are marked with a leading "# " and list items with "• ". Headings left without content are removed.
func CleanReadme(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = mdComment.ReplaceAllString(md, "")
	md = mdHTMLHead.ReplaceAllString(md, "\n# $1\n")
	md = mdHTMLItem.ReplaceAllString(md, "\n- ")
	md = mdHTMLBreak.ReplaceAllString(md, "\n")
	md = mdHTMLTag.ReplaceAllString(md, "")
	md = mdImage.ReplaceAllString(md, "")
	md = mdLink.ReplaceAllString(md, "$1$2")
	md = mdEmphasis.ReplaceAllString(md, "$1$2$3")
	md = html.UnescapeString(md)

	var lines []string
	fence := false
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			lines = append(lines, "  "+strings.TrimRight(l, " \t"))
			continue
		}
		l = strings.TrimSpace(strings.ReplaceAll(l, "`", ""))
		switch {
		case mdRefDef.MatchString(l), mdTableRule.MatchString(l), mdRule.MatchString(l):
			continue
		case mdHeading.MatchString(l):
			l = "# " + strings.TrimSpace(mdHeading.ReplaceAllString(l, ""))
		case mdBullet.MatchString(l):
			l = "• " + mdBullet.ReplaceAllString(l, "")
		case strings.HasPrefix(l, "|"):
			cells := strings.FieldsFunc(l, func(r rune) bool { return r == '|' })
			for i := range cells {
				cells[i] = strings.TrimSpace(cells[i])
			}
			l = strings.Join(cells, " · ")
		case strings.HasPrefix(l, ">"):
			l = strings.TrimSpace(strings.TrimLeft(l, "> "))
		}
		if strings.Trim(l, "#•· ") == "" {
			l = ""
		}
		lines = append(lines, l)
	}

	// Drop headings whose section ended up empty, then collapse runs of blank lines.
	var out []string
	for i, l := range lines {
		if strings.HasPrefix(l, "# ") {
			next := ""
			for _, n := range lines[i+1:] {
				if n != "" {
					next = n
					break
				}
			}
			if next == "" || strings.HasPrefix(next, "# ") {
				continue
			}
		}
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Identicon draws a GitHub-style default avatar for login: a mirrored 5×5 pattern in one color on a
// light background, size×size pixels.
func Identicon(login string, size int) *Pixels {
	sum := md5.Sum([]byte(login))
	// Hue from the hash, fixed saturation and lightness, like github.com's identicons.
	r, g, b := hslToRGB(float64(int(sum[12])<<4|int(sum[13])>>4)/4096, 0.55, 0.55)
	px := &Pixels{W: size, H: size, RGB: make([]byte, size*size*3)}
	margin := size / 12
	cell := (size - 2*margin) / 5
	off := (size - 5*cell) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			cr, cg, cb := byte(0xf0), byte(0xf0), byte(0xf0)
			cx, cy := (x-off)/cell, (y-off)/cell
			if x >= off && y >= off && cx < 5 && cy < 5 {
				col := cx
				if col > 2 {
					col = 4 - col
				}
				if sum[(col*5+cy)%15]%2 == 0 {
					cr, cg, cb = r, g, b
				}
			}
			i := 3 * (y*size + x)
			px.RGB[i], px.RGB[i+1], px.RGB[i+2] = cr, cg, cb
		}
	}
	return px
}

func hslToRGB(h, s, l float64) (r, g, b byte) {
	hue := func(p, q, t float64) float64 {
		switch {
		case t < 0:
			t++
		case t > 1:
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	return byte(hue(p, q, h+1.0/3) * 255), byte(hue(p, q, h) * 255), byte(hue(p, q, h-1.0/3) * 255)
}
