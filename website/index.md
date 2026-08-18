---
layout: home
---

<div class="edikt-home">
<div class="ledger">
<div class="rail"></div>
<div class="content">

<section class="hero">
  <span class="eyebrow">Architecture governance for agentic engineering</span>
  <h1>Your coding standards, enforced. Your decisions, remembered.</h1>
  <p class="lede">Capture an architecture decision once. edikt compiles it into a directive the model follows automatically — every session, every engineer, without you saying it twice.</p>
  <div class="ctas">
    <a class="btn btn-primary" href="/getting-started">Get started — 5 minutes →</a>
    <a class="btn btn-ghost" href="/what-is-edikt">What is edikt?</a>
  </div>
  <div class="install-block" id="install">
    <div class="install-tabs" role="tablist" aria-label="Install method">
      <button class="install-tab" type="button" role="tab" aria-selected="true" aria-controls="install-curl" data-tab="curl">curl</button>
      <button class="install-tab" type="button" role="tab" aria-selected="false" aria-controls="install-brew" data-tab="brew">brew</button>
      <button class="install-copy" type="button" aria-label="Copy install command">Copy</button>
    </div>
    <div class="install-code-row">
      <code id="install-curl" role="tabpanel"><span class="prompt">$</span> curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.7.1/install.sh | bash</code>
      <code id="install-brew" role="tabpanel" hidden><span class="prompt">$</span> brew install diktahq/tap/edikt</code>
    </div>
  </div>
  <p class="harness-note"><span class="now">Claude Code</span> today — Codex and other harnesses next.</p>
</section>

<section class="replay" id="replay">
  <span class="eyebrow">See it work</span>
  <h2>A decision that survives three weeks and a team change</h2>

  <div class="replay-grid">
    <ol class="beats">
      <li>
        <span class="beat-num">1</span>
        <div><strong>You decide, once.</strong><span class="beat-text">"Postgres for the orders table, pooling in the app." Said in plain language — saved as <span class="adr-ref">ADR-014</span>.</span></div>
      </li>
      <li>
        <span class="beat-num">2</span>
        <div><strong>edikt remembers.</strong><span class="beat-text">Every session in this repo reads it automatically from here on — no re-explaining, no wiki to check.</span></div>
      </li>
      <li>
        <span class="beat-num">3</span>
        <div><strong>Someone tries to change it — the write gets blocked.</strong><span class="beat-text">Three weeks later, a different engineer edits the database config toward Mongo. edikt denies the write before it lands, citing <span class="adr-ref">ADR-014</span> by name.</span></div>
      </li>
    </ol>
    <div class="term-col">
      <p class="term-label">Watch it happen</p>
      <div class="term">
        <div class="term-bar">
          <span class="term-dot"></span><span class="term-dot"></span><span class="term-dot"></span>
          <span class="term-title">claude — orders-service</span>
        </div>
        <div class="term-body" id="termBody"></div>
      </div>
      <div class="replay-foot">
        <button type="button" id="replayBtn">Replay</button>
        <span class="note">Plays once automatically when scrolled into view.</span>
      </div>
    </div>
  </div>
</section>

<section class="problem reveal">
  <span class="eyebrow">The gap</span>
  <p>The model has memory. <span class="fade">It doesn't have governance.</span><br>Every session starts from zero — and a decision your team made yesterday gets quietly contradicted today.</p>
</section>

<section class="clauses">
<span class="eyebrow clauses-label">What edikt does</span>

<article class="clause reveal">
  <h2>Decisions stop living only in your head</h2>
  <p class="benefit">Write an architecture decision as plain prose. edikt compiles it into a directive the model reads automatically, before it writes a line of code — no re-explaining, no pinned Slack message, no wiki nobody opens.</p>
  <p class="detail">/edikt:gov:compile → reads accepted decisions, checks for contradictions between them, renders enforcement. Update one, recompile. <span class="must">One source of truth</span>, not a doc that drifts from the code.</p>
</article>

<article class="clause reveal">
  <h2>A hardcoded secret never reaches your build</h2>
  <p class="benefit">Critical findings block the agent automatically — before the code lands, not after review. If someone overrides a gate, that's logged with their identity, so trade-offs are visible, not silent.</p>
  <p class="detail">Security, DBA, and architecture review run inline, on every change. <span class="must">No silent failures.</span></p>
</article>

<article class="clause reveal">
  <h2>You talk. edikt handles the rest</h2>
  <p class="benefit">Ask "what's our status?" and get the governance dashboard. Say "save this decision" and it's captured, compiled, and enforced from the next session on. No command syntax to memorize.</p>
  <p class="detail">No slash commands to learn — describe what you want in plain English. <span class="must">Same vocabulary you already use.</span></p>
</article>

<article class="clause reveal">
  <h2>Nothing to run, nothing to maintain</h2>
  <p class="benefit">Every file is markdown or YAML, committed to your repo like any other source file. No daemon, no build step, no vendor lock-in — stop using edikt and the files stay exactly as readable as they were.</p>
  <p class="detail">git diff shows exactly what changed, same as any other commit. <span class="must">Nothing hidden, nothing binary.</span></p>
</article>

</section>

<section class="segment reveal">
  <div class="card">
    <span class="eyebrow who">For teams running Claude Code together</span>
    <p>One engineer installs edikt and commits it. Every teammate's session inherits the same standards from that point on — code review stops catching formatting and starts catching design decisions, whether it's your best engineer's session or the one who joined this week.</p>
    <p class="mechanism">The rules live as markdown and YAML in the repo, committed like any other source file — the model reads them automatically for every teammate, at the start of every session, no separate setup step.</p>
    <div class="roster">
      <span class="roster-avatar">JD</span>
      <span class="roster-avatar">MK</span>
      <span class="roster-avatar new">+1</span>
      <span class="roster-caption">Same <span class="adr-chip">ADR-014</span>, every one of their sessions — including the engineer who joined this week.</span>
    </div>
  </div>
  <p class="secondary"><strong>Working solo?</strong> Same mechanism, just you and the model staying in sync from one session to the next — no re-explaining your own architecture back to yourself. <strong>Running a consultancy?</strong> Install once, and every client engagement inherits the same standards from day one.</p>
</section>

<section class="proof reveal">
  <span class="chip"><b>v0.7.1</b> · Cosign-signed releases</span>
  <span class="chip"><b>Elastic-2.0</b> · Open source</span>
  <span class="chip"><b>No build step</b> · No npm, no compiler</span>
  <span class="chip"><b>Plain markdown</b> · You own every file</span>
</section>

<section class="closing reveal">
  <span class="eyebrow">Start here</span>
  <h2>Five minutes, and the model stops forgetting what you decided.</h2>
  <div class="ctas">
    <a class="btn btn-primary" href="/getting-started">Get started — 5 minutes →</a>
    <a class="btn btn-ghost" href="/what-is-edikt">What is edikt?</a>
  </div>
</section>

</div>
</div>
</div>

<style>
body:has(.VPHome) .VPNavBarSearch { display: none !important; }

.edikt-home {
  max-width: 72rem;
  margin: 0 auto;
  padding: 32px 24px 64px;
}
.edikt-home :where(h1, h2, p, span, article, div) { margin: 0; }
.edikt-home a { text-decoration: none; }

.edikt-home .ledger {
  display: grid;
  grid-template-columns: 5.5rem minmax(0, 1fr);
  gap: 0 2rem;
}
.edikt-home .ledger > .rail {
  border-right: 1px solid var(--vp-c-divider);
  padding-top: 0.35rem;
  position: relative;
}
.edikt-home .ledger > .rail::after {
  content: '';
  position: absolute;
  top: 0; right: -1px;
  width: 2px;
  height: 100%;
  background: var(--vp-c-brand-1);
  transform: scaleY(var(--progress, 0));
  transform-origin: top;
}
.edikt-home .ledger > .content { min-width: 0; }
@media (prefers-reduced-motion: no-preference) {
  .edikt-home .ledger > .rail::after { transition: transform 100ms linear; }
}
@media (max-width: 760px) {
  .edikt-home .ledger { grid-template-columns: 1fr; }
  .edikt-home .ledger > .rail {
    border-right: none;
    border-bottom: 1px solid var(--vp-c-divider);
    padding-bottom: 0.5rem;
    margin-bottom: 1.25rem;
  }
  .edikt-home .ledger > .rail::after { display: none; }
}

/* ---------- Scroll reveal ---------- */
.reveal {
  opacity: 0;
  transform: translateY(14px);
  transition: opacity 560ms ease, transform 560ms ease;
}
.reveal.in { opacity: 1; transform: none; }
@media (prefers-reduced-motion: reduce) {
  .reveal { opacity: 1; transform: none; transition: none; }
}

.edikt-home .eyebrow {
  display: flex;
  align-items: center;
  gap: 0.6em;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.11em;
  text-transform: uppercase;
  color: var(--vp-c-brand-1);
}
.edikt-home .eyebrow::before {
  content: '';
  flex: none;
  width: 22px;
  height: 2px;
  background: var(--vp-c-brand-1);
}

/* ---------- Hero ---------- */
.edikt-home .hero { padding: 3.5rem 0 4rem; }
.edikt-home .hero .eyebrow { margin-bottom: 1.1rem; }
.edikt-home .hero h1 {
  font-family: 'Space Grotesk', var(--vp-font-family-base);
  font-size: clamp(2.4rem, 5.2vw, 4rem);
  font-weight: 700;
  line-height: 1.08;
  letter-spacing: -0.02em;
  text-wrap: balance;
  max-width: 20ch;
  color: var(--vp-c-text-1);
}
.edikt-home .hero .lede {
  margin-top: 1.6rem !important;
  max-width: 42ch;
  font-size: 1.2rem;
  line-height: 1.55;
  color: var(--vp-c-text-2);
}
.edikt-home .hero .ctas {
  margin-top: 2.25rem;
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
  align-items: center;
}
.edikt-home .hero .btn,
.edikt-home .closing .btn {
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 0.85rem;
  letter-spacing: 0.02em;
  padding: 0.85rem 1.4rem;
  border: 1px solid var(--vp-c-text-1);
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
  text-decoration: none;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;
}
.edikt-home .hero .btn-primary,
.edikt-home .closing .btn-primary { background: var(--vp-c-text-1); color: var(--vp-c-bg); }
.edikt-home .hero .btn-primary:hover,
.edikt-home .closing .btn-primary:hover { background: var(--vp-c-brand-1); border-color: var(--vp-c-brand-1); color: #fff; }
.edikt-home .hero .btn-ghost,
.edikt-home .closing .btn-ghost { color: var(--vp-c-text-2); border-color: var(--vp-c-divider); }
.edikt-home .hero .btn-ghost:hover,
.edikt-home .closing .btn-ghost:hover { color: var(--vp-c-text-1); border-color: var(--vp-c-text-2); }

.edikt-home .install-block {
  margin-top: 2.75rem;
  border: 1px solid var(--vp-c-divider);
  border-radius: 4px;
  background: var(--vp-c-bg-alt);
  max-width: 100%;
  width: fit-content;
  min-width: min(100%, 34rem);
}
.edikt-home .install-tabs {
  display: flex;
  border-bottom: 1px solid var(--vp-c-divider);
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
}
.edikt-home .install-tab {
  appearance: none;
  background: none;
  border: none;
  border-right: 1px solid var(--vp-c-divider);
  padding: 8px 16px;
  font-family: inherit;
  font-size: 12px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--vp-c-text-2);
  cursor: pointer;
}
.edikt-home .install-tab[aria-selected="true"] {
  color: var(--vp-c-brand-1);
  background: var(--vp-c-brand-soft);
}
.edikt-home .install-tab:hover:not([aria-selected="true"]) { color: var(--vp-c-text-2); }
.edikt-home .install-copy {
  appearance: none;
  background: none;
  border: none;
  margin-left: auto;
  padding: 8px 16px;
  font-family: inherit;
  font-size: 12px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--vp-c-text-2);
  cursor: pointer;
}
.edikt-home .install-copy:hover { color: var(--vp-c-text-1); }
.edikt-home .install-copy.copied { color: var(--vp-c-brand-1); }
.edikt-home .install-code-row { overflow-x: auto; }
.edikt-home .install-code-row code {
  display: block;
  width: max-content;
  min-width: 100%;
  padding: 14px 16px;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 14px;
  color: var(--vp-c-text-1);
  white-space: pre;
  background: none;
}
.edikt-home .install-code-row .prompt { color: var(--vp-c-brand-1); }
.edikt-home .install-code-row code[hidden] { display: none; }
@media (max-width: 520px) {
  .edikt-home .install-code-row code { font-size: 12.5px; }
}

.edikt-home .harness-note {
  margin-top: 0.85rem !important;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--vp-c-text-2);
}
.edikt-home .harness-note .now { color: var(--vp-c-text-2); font-weight: 500; }

/* ---------- Problem statement ---------- */
.edikt-home .problem {
  padding: 4rem 0 0;
}
.edikt-home .problem .eyebrow { margin-bottom: 1rem; }
.edikt-home .problem p {
  font-family: 'Space Grotesk', var(--vp-font-family-base);
  font-size: 1.5rem;
  font-weight: 600;
  line-height: 1.4;
  max-width: 46ch;
  color: var(--vp-c-text-1);
}
.edikt-home .problem .fade { color: var(--vp-c-text-2); }

/* ---------- Clauses ---------- */
.edikt-home .clauses { padding-top: 3.5rem; }
.edikt-home .clauses-label { margin-bottom: 0.9rem; }
.edikt-home .clause {
  display: block;
  padding: 1.75rem 1.25rem;
  margin: 0 -1.25rem;
  border-radius: 6px;
  transition: background 180ms ease;
}
.edikt-home .clause:first-child { padding-top: 0; margin-top: 0; }
.edikt-home .clause + .clause { border-top: 1px solid var(--vp-c-divider); }
.edikt-home .clause:hover { background: var(--vp-c-bg-soft); }
.edikt-home .clause h2 {
  font-family: 'Space Grotesk', var(--vp-font-family-base);
  font-size: clamp(1.4rem, 2.4vw, 1.75rem);
  font-weight: 600;
  line-height: 1.25;
  letter-spacing: -0.01em;
  max-width: 26ch;
  color: var(--vp-c-text-1);
  border-top: none;
  padding-top: 0;
}
.edikt-home .clause .benefit {
  margin-top: 0.9rem !important;
  font-size: 1.05rem;
  line-height: 1.6;
  color: var(--vp-c-text-2);
  max-width: 46ch;
}
.edikt-home .clause .detail {
  display: block;
  margin-top: 1rem !important;
  padding: 0.6rem 0 0.6rem 0.9rem;
  border-left: 2px solid var(--vp-c-divider);
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 0.8rem;
  line-height: 1.65;
  color: var(--vp-c-text-2);
  max-width: 52ch;
}
.edikt-home .clause .detail .must { color: var(--vp-c-brand-1); }

/* ---------- Segment ---------- */
.edikt-home .segment { padding: 3rem 0 3.5rem; }
.edikt-home .segment .card {
  background: var(--vp-c-bg-alt);
  border: 1px solid var(--vp-c-divider);
  border-radius: 4px;
  padding: 28px 32px;
  max-width: 42rem;
}
.edikt-home .segment .card .who { margin-bottom: 12px !important; }
.edikt-home .segment .card p { font-size: 1.1rem; line-height: 1.55; max-width: 44ch; color: var(--vp-c-text-1); }
.edikt-home .segment .card p.mechanism {
  margin-top: 1rem !important;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 0.8rem;
  line-height: 1.65;
  color: var(--vp-c-text-2);
  max-width: 48ch;
}
.edikt-home .segment .secondary {
  margin-top: 1.5rem !important;
  font-size: 0.95rem;
  line-height: 1.6;
  color: var(--vp-c-text-2);
  max-width: 44ch;
}
.edikt-home .segment .secondary strong { color: var(--vp-c-text-2); font-weight: 600; }

.edikt-home .roster {
  margin-top: 1.25rem !important;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.6rem 0.7rem;
  padding-top: 1.1rem;
  border-top: 1px solid var(--vp-c-divider);
}
.edikt-home .roster-avatar {
  flex: none;
  width: 28px; height: 28px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 10px; font-weight: 700;
  color: var(--vp-c-text-2);
  opacity: 0; transform: scale(0.6);
  transition: opacity 360ms ease, transform 360ms ease;
}
.edikt-home .segment.in .roster-avatar { opacity: 1; transform: none; }
.edikt-home .segment.in .roster-avatar:nth-of-type(1) { transition-delay: 80ms; }
.edikt-home .segment.in .roster-avatar:nth-of-type(2) { transition-delay: 160ms; }
.edikt-home .segment.in .roster-avatar:nth-of-type(3) { transition-delay: 240ms; }
.edikt-home .roster-avatar.new { background: var(--vp-c-brand-soft); border-color: var(--vp-c-brand-1); color: var(--vp-c-brand-1); }
.edikt-home .roster-caption { font-size: 0.85rem; line-height: 1.5; color: var(--vp-c-text-2); max-width: 34ch; }
.edikt-home .adr-chip {
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 0.85em; font-weight: 700; color: var(--vp-c-brand-1);
}
@media (prefers-reduced-motion: reduce) {
  .edikt-home .roster-avatar { opacity: 1; transform: none; }
}

/* ---------- Proof strip ---------- */
.edikt-home .proof {
  padding: 1.5rem 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 0.7rem;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono);
  font-size: 12.5px;
  color: var(--vp-c-text-2);
}
.edikt-home .proof .chip {
  border: 1px solid var(--vp-c-divider);
  border-radius: 999px;
  padding: 6px 14px;
  opacity: 0; transform: translateY(6px);
  transition: opacity 360ms ease, transform 360ms ease;
}
.edikt-home .proof.in .chip { opacity: 1; transform: none; }
.edikt-home .proof.in .chip:nth-child(1) { transition-delay: 0ms; }
.edikt-home .proof.in .chip:nth-child(2) { transition-delay: 70ms; }
.edikt-home .proof.in .chip:nth-child(3) { transition-delay: 140ms; }
.edikt-home .proof.in .chip:nth-child(4) { transition-delay: 210ms; }
.edikt-home .proof b { color: var(--vp-c-text-1); font-weight: 700; }
@media (prefers-reduced-motion: reduce) {
  .edikt-home .proof .chip { opacity: 1; transform: none; }
}

/* ---------- Closing ---------- */
.edikt-home .closing {
  margin-top: 3rem;
  background: var(--vp-c-bg-alt);
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  padding: 3rem 2.5rem;
}
.edikt-home .closing .eyebrow { margin-bottom: 1rem; }
.edikt-home .closing h2 {
  font-family: 'Space Grotesk', var(--vp-font-family-base);
  font-size: clamp(1.75rem, 3.6vw, 2.4rem);
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.02em;
  text-wrap: balance;
  max-width: 18ch;
  color: var(--vp-c-text-1);
  border-top: none;
  padding-top: 0;
}
.edikt-home .closing .ctas { margin-top: 2rem; display: flex; gap: 1rem; flex-wrap: wrap; }

/* ---------- Replay demo ---------- */
.edikt-home .replay { padding: 4rem 0 0; }
.edikt-home .replay .eyebrow { margin-bottom: 0.9rem; }
.edikt-home .replay h2 {
  font-family: 'Space Grotesk', var(--vp-font-family-base);
  font-size: clamp(1.6rem, 3vw, 2.1rem);
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--vp-c-text-1);
  max-width: 30ch;
}
.edikt-home .replay-grid { display: grid; grid-template-columns: minmax(0,21rem) minmax(0,1fr); gap: 2.75rem; align-items: start; margin-top: 2rem; }
@media (max-width: 820px) { .edikt-home .replay-grid { grid-template-columns: 1fr; gap: 2rem; } }
.edikt-home .beats { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1.5rem; }
.edikt-home .beats li { display: flex; gap: 0.9rem; align-items: flex-start; }
.edikt-home .beat-num {
  flex: none; width: 26px; height: 26px; border-radius: 50%;
  background: var(--vp-c-brand-soft); color: var(--vp-c-brand-1);
  font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 12px; font-weight: 700;
  display: flex; align-items: center; justify-content: center; margin-top: 0.1rem;
}
.edikt-home .beats strong {
  display: block; font-family: 'Space Grotesk', var(--vp-font-family-base); font-size: 1.02rem; font-weight: 600;
  color: var(--vp-c-text-1); margin-bottom: 0.25rem; letter-spacing: -0.005em;
}
.edikt-home .beats span.beat-text { display: block; font-size: 0.95rem; line-height: 1.55; color: var(--vp-c-text-2); }
.edikt-home .beats .adr-ref { color: var(--vp-c-brand-1); font-weight: 600; font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 0.88em; }
.edikt-home .term-label {
  font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 11px; font-weight: 700; letter-spacing: 0.09em;
  text-transform: uppercase; color: var(--vp-c-text-2); margin: 0 0 0.7rem;
}
.edikt-home .term {
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background: var(--vp-c-bg-alt);
  overflow: hidden;
}
.edikt-home .term-bar {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 14px; border-bottom: 1px solid var(--vp-c-divider);
}
.edikt-home .term-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--vp-c-divider); }
.edikt-home .term-title {
  margin-left: 8px; font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 11px; letter-spacing: 0.06em;
  text-transform: uppercase; color: var(--vp-c-text-2);
}
.edikt-home .term-body {
  padding: 20px 18px 24px;
  font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 13.5px; line-height: 1.75;
  min-height: 300px;
}
.edikt-home .term-line {
  opacity: 0; transform: translateY(6px);
  transition: opacity 420ms ease, transform 420ms ease;
  margin-bottom: 2px;
  white-space: pre-wrap;
}
.edikt-home .term-line.in { opacity: 1; transform: none; }
.edikt-home .term-line .prompt-mark { color: var(--vp-c-brand-1); margin-right: 0.5em; }
.edikt-home .term-line .you { color: var(--vp-c-text-1); }
.edikt-home .term-line .claude-label { color: var(--vp-c-brand-1); font-weight: 600; margin-right: 0.6em; }
.edikt-home .term-line .claude { color: var(--vp-c-text-2); }
.edikt-home .term-line.divider {
  color: var(--vp-c-text-3); font-size: 11.5px; letter-spacing: 0.04em;
  margin: 14px 0; text-align: center;
}
.edikt-home .term-line .adr-ref { color: var(--vp-c-brand-1); font-weight: 600; }
.edikt-home .term-line.block {
  margin-top: 10px;
  padding: 10px 12px;
  border-left: 2px solid var(--vp-c-brand-1);
  background: var(--vp-c-bg);
}
.edikt-home .term-line .block-label {
  display: block;
  font-size: 10.5px; font-weight: 700; letter-spacing: 0.07em; text-transform: uppercase;
  color: var(--vp-c-brand-1); margin-bottom: 5px;
}
.edikt-home .term-line .block-body { display: block; color: var(--vp-c-text-2); }
.edikt-home .term-line .block-body .path { color: var(--vp-c-text-1); font-weight: 600; }
.edikt-home .term-cursor {
  display: inline-block; width: 7px; height: 1em; background: var(--vp-c-brand-1);
  vertical-align: text-bottom; opacity: 0; animation: edikt-blink 1s steps(1) infinite;
}
.edikt-home .term-cursor.show { opacity: 1; }
@keyframes edikt-blink { 50% { opacity: 0; } }
.edikt-home .replay-foot { display: flex; align-items: center; gap: 1.2rem; margin-top: 1.2rem; flex-wrap: wrap; }
.edikt-home .replay-foot button {
  appearance: none; background: none; border: 1px solid var(--vp-c-divider); border-radius: 4px;
  padding: 7px 14px; font-family: 'JetBrains Mono', var(--vp-font-family-mono); font-size: 12px; letter-spacing: 0.04em;
  text-transform: uppercase; color: var(--vp-c-text-2); cursor: pointer;
}
.edikt-home .replay-foot button:hover { color: var(--vp-c-text-1); border-color: var(--vp-c-text-2); }
.edikt-home .replay-foot .note { font-size: 13px; color: var(--vp-c-text-3); }
</style>

<script>
if (typeof document !== 'undefined') {
  var wireInstallTabs = function () {
    var tabs = document.querySelectorAll('.install-tab');
    if (!tabs.length) return;
    tabs.forEach(function (tab) {
      if (tab.dataset.wired) return;
      tab.dataset.wired = '1';
      tab.addEventListener('click', function () {
        tabs.forEach(function (t) { t.setAttribute('aria-selected', 'false'); });
        tab.setAttribute('aria-selected', 'true');
        document.querySelectorAll('.install-code-row [role="tabpanel"]').forEach(function (p) { p.hidden = true; });
        document.getElementById('install-' + tab.dataset.tab).hidden = false;
      });
    });
    var copyBtn = document.querySelector('.install-copy');
    if (copyBtn && !copyBtn.dataset.wired) {
      copyBtn.dataset.wired = '1';
      copyBtn.addEventListener('click', function () {
        var active = document.querySelector('.install-code-row [role="tabpanel"]:not([hidden])');
        if (!active) return;
        var text = active.textContent.replace(/^\s*\$\s*/, '').trim();
        navigator.clipboard.writeText(text).then(function () {
          copyBtn.textContent = 'Copied';
          copyBtn.classList.add('copied');
          setTimeout(function () {
            copyBtn.textContent = 'Copy';
            copyBtn.classList.remove('copied');
          }, 1500);
        });
      });
    }
  };
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireInstallTabs);
  } else {
    wireInstallTabs();
  }
}

(function () {
  if (typeof document === 'undefined') return;
  var lines = [
    { t: 'you', text: 'We\'re using Postgres for the orders table. Connection pooling stays at the app layer — no pgbouncer.' },
    { t: 'claude', text: 'Got it. Want this saved as a decision?' },
    { t: 'you', text: 'save this decision' },
    { t: 'claude', html: 'Saved as <span class="adr-ref">ADR-014</span>. Compiling governance… done.\nEvery session in this repo reads it from here on.' },
    { t: 'divider', text: '———  3 weeks later · a different engineer  ———' },
    { t: 'you', text: 'edit config/database.yml — switch the orders table to MongoDB' },
    { t: 'block', html: 'This write touches <span class="path">config/database.yml</span>, governed by 1 MUST-grade directive:\n— Postgres for the orders table, pooling at the app layer. (<span class="adr-ref">ADR-014</span>)\nNon-negotiable. Revise the write to comply, then proceed.' }
  ];

  var body = document.getElementById('termBody');
  var replayBtn = document.getElementById('replayBtn');
  var reduceMotion = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var playing = false;
  if (!body) return;

  function buildLineEl(line) {
    var div = document.createElement('div');
    div.className = 'term-line';
    if (line.t === 'divider') {
      div.className += ' divider';
      div.textContent = line.text;
      return div;
    }
    if (line.t === 'you') {
      div.innerHTML = '<span class="prompt-mark">&gt;</span><span class="you"></span>';
      div.querySelector('.you').textContent = line.text;
      return div;
    }
    if (line.t === 'block') {
      div.className += ' block';
      div.innerHTML = '<span class="block-label">edikt — write denied</span><span class="block-body"></span>';
      var body_ = div.querySelector('.block-body');
      if (line.html) { body_.innerHTML = line.html; } else { body_.textContent = line.text; }
      return div;
    }
    div.innerHTML = '<span class="claude-label">Claude</span><span class="claude"></span>';
    if (line.html) {
      div.querySelector('.claude').innerHTML = line.html;
    } else {
      div.querySelector('.claude').textContent = line.text;
    }
    return div;
  }

  function play() {
    if (playing) return;
    playing = true;
    body.innerHTML = '';
    var cursor = document.createElement('span');
    cursor.className = 'term-cursor';
    body.appendChild(cursor);

    if (reduceMotion) {
      lines.forEach(function (line) {
        var el = buildLineEl(line);
        el.classList.add('in');
        body.insertBefore(el, cursor);
      });
      cursor.classList.add('show');
      playing = false;
      return;
    }

    var i = 0;
    function step() {
      if (i >= lines.length) { cursor.classList.add('show'); playing = false; return; }
      var el = buildLineEl(lines[i]);
      body.insertBefore(el, cursor);
      window.requestAnimationFrame(function () { el.classList.add('in'); });
      i++;
      window.setTimeout(step, lines[i - 1].t === 'divider' ? 380 : 620);
    }
    step();
  }

  if (replayBtn) replayBtn.addEventListener('click', play);

  if ('IntersectionObserver' in window) {
    var seen = false;
    var obs = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting && !seen) { seen = true; play(); }
      });
    }, { threshold: 0.4 });
    var target = document.getElementById('replay');
    if (target) obs.observe(target);
  } else {
    play();
  }
})();

(function () {
  if (typeof document === 'undefined') return;
  var els = document.querySelectorAll('.reveal');
  if (!els.length) return;
  var reduceMotion = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (reduceMotion || !('IntersectionObserver' in window)) {
    els.forEach(function (el) { el.classList.add('in'); });
    return;
  }
  var obs = new IntersectionObserver(function (entries) {
    entries.forEach(function (entry) {
      if (entry.isIntersecting) {
        entry.target.classList.add('in');
        obs.unobserve(entry.target);
      }
    });
  }, { threshold: 0.15, rootMargin: '0px 0px -40px 0px' });
  els.forEach(function (el) { obs.observe(el); });
})();

(function () {
  if (typeof document === 'undefined') return;
  var rail = document.querySelector('.edikt-home .ledger > .rail');
  var content = document.querySelector('.edikt-home .ledger > .content');
  if (!rail || !content) return;
  function update() {
    var rect = content.getBoundingClientRect();
    var total = rect.height - window.innerHeight;
    var scrolled = -rect.top;
    var progress = total > 0 ? Math.min(1, Math.max(0, scrolled / total)) : 0;
    rail.style.setProperty('--progress', progress.toFixed(3));
  }
  window.addEventListener('scroll', update, { passive: true });
  window.addEventListener('resize', update);
  update();
})();
</script>
