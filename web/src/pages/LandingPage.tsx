import { Link } from "react-router-dom";

export default function LandingPage() {
  return (
    <div className="landing">
      <div className="landing-hero">
        <h1>
          <span className="landing-logo">Gitwise</span>
        </h1>
        <p className="landing-tagline">
          AI-native code collaboration, self-hosted.
        </p>
        <div className="landing-cta">
          <Link to="/register" className="btn btn-primary btn-lg">
            Get started
          </Link>
          <Link to="/login" className="btn btn-secondary btn-lg">
            Sign in
          </Link>
        </div>
      </div>

      <div className="landing-features">
        <div className="landing-feature">
          <h3>Git hosting</h3>
          <p>Push, pull, branch, and browse repositories. Full HTTP git protocol support.</p>
        </div>
        <div className="landing-feature">
          <h3>Issues &amp; PRs</h3>
          <p>Track bugs, plan features, review code. Everything you need to collaborate.</p>
        </div>
        <div className="landing-feature">
          <h3>Webhooks</h3>
          <p>Discord-native webhook embeds. Connect your repos to the tools you use.</p>
        </div>
        <div className="landing-feature">
          <h3>Self-hosted</h3>
          <p>Your code, your server. No vendor lock-in, no data harvesting, no limits.</p>
        </div>
      </div>

      <div className="landing-note">
        <p>
          I love GitHub and am not seeking to replace it &mdash; I just want to try
          to integrate AI as deeply as possible into the development workflow, for love
          of the game. Gitwise is an experiment in what code collaboration looks like
          when AI is a first-class citizen, not an afterthought.
        </p>
        <p className="landing-author">&mdash; Dylan</p>
      </div>
    </div>
  );
}
