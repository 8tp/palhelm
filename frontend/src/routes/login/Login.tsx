import { useState, type FormEvent } from "react";
import { Navigate } from "react-router";
import { useAuth } from "../../app/AuthProvider";
import { HelmMark } from "../../components/icons";
import "./Login.css";


export default function Login() {
  const { status, login, loginError, loggingIn } = useAuth();
  const [password, setPassword] = useState("");

  if (status === "authenticated") {
    return <Navigate to="/" replace />;
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!password) return;
    try {
      await login(password);
    } catch {
      // loginError is surfaced via context state
    }
  }

  return (
    <div className="login-page">
      {/* the expedition map on the desk: full-bleed hero, dims to a lantern-lit chart at night camp */}
      <div className="desk" aria-hidden="true" />

      <div className="login-wrap">
        <div className="login-brand">
          <HelmMark size={72} strokeWidth={1.6} dotRadius={1.1} wheelClassName="wheel" className="helm-mark" />
          <span className="word">palhelm</span>
          <span className="tagline">take the helm of your Palworld server</span>
        </div>

        <form className="card login-card" onSubmit={onSubmit}>
          <div>
            <h1>Sign in</h1>
            <p className="server">My Palworld Server</p>
          </div>
          {loginError && <p className="form-error">{loginError}</p>}
          <div className="field">
            <label htmlFor="pw">Password</label>
            <input
              className="input"
              id="pw"
              type="password"
              autoComplete="current-password"
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <button className="btn btn-primary" type="submit" style={{ justifyContent: "center" }} disabled={loggingIn}>
            {loggingIn ? "Signing in…" : "Sign in"}
          </button>
          <p style={{ fontSize: "var(--text-xs)", color: "var(--ink-2)" }}>
            Admin and viewer passwords are set by the server operator via environment variables.
          </p>
        </form>

        <div className="login-foot">
          <span>Palhelm server administration</span>
          <a href="https://github.com/8tp/palhelm" target="_blank" rel="noreferrer">
            open source
          </a>
          <a href="https://docs.palhelm.com" target="_blank" rel="noreferrer">
            docs
          </a>
        </div>
      </div>
    </div>
  );
}
