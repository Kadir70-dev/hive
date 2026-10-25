'use client';
import Link from "next/link";

// DRAFT for owner and legal review. Every statement below describes how the
// code in this repository actually behaves; items in [brackets] need an owner
// decision before publication. Do not present this as legally reviewed.
const sections: { title: string; body: string[]; list?: string[] }[] = [
  {
    title: "Who we are",
    body: [
      "The Hive Society is a community for women and men of all nationalities in Abu Dhabi, United Arab Emirates. [Legal entity name and registered address to be confirmed.] For anything about your information, email hello@thehivesociety.ae.",
    ],
  },
  {
    title: "What we collect",
    body: ["When you create a members' account and use the site we store:"],
    list: [
      "Your first name and email address, and optionally a display name, phone number, area of Abu Dhabi and interests",
      "Your password, stored only as a one-way Argon2id hash that nobody, including us, can read",
      "Your membership status, when you confirmed your email address, and whether you want optional email updates",
      "The gatherings you keep a place for or join the waitlist for, and attendance recorded by organisers",
      "Safety reports you send: the concern, an optional gathering, your description of the person involved and whether you asked not to be booked with them",
      "If you joined the earlier founding community list: your name, email, optional phone number, note and language",
      "Security records: sign-ins, email confirmations, password resets, two-factor events for administrators and administrator actions. These never contain your password",
    ],
  },
  {
    title: "How we use it",
    body: ["We use your information only to run The Hive Society:"],
    list: [
      "To create and secure your account, confirm your email address and let you reset a forgotten password",
      "To review membership applications personally. When you confirm your email, our administrators receive an alert with your first name and area",
      "To manage gatherings: places, the waitlist (the earliest person in line is offered a freed place automatically) and attendance",
      "To send service emails: confirming your email, password resets, password-change notices, membership approval, booking confirmations, cancellations, waitlist promotions, and changes to or cancellation of gatherings you reserved",
      "To keep the community safe: our administrators review reports, may identify the member concerned from the gathering list, and can keep two members from being booked into the same gathering. The member concerned is never told who reported",
      "Announcements by email only if you switch on email updates in your profile; you can switch them off at any time",
    ],
  },
  {
    title: "Who can see it",
    body: [
      "Other members cannot see your email address, phone number, attendee identity or the gatherings you attend; there is no member directory or member-to-member messaging.",
      "Only our administrators can see member details, to review applications and know who is coming to each gathering. Administrator access requires a confirmed email address and two-factor authentication.",
      "Partners listed on the site receive no member information.",
    ],
  },
  {
    title: "Service providers",
    body: [
      "We do not sell your information and we do not share it with advertisers or data brokers. We use these providers to operate the service:",
    ],
    list: [
      "Hetzner Online — servers that run our application, database and backups [data-centre location to be confirmed]",
      "Netlify — hosting for this website; your requests pass through Netlify to reach our servers",
      "Resend — delivery of the service emails listed above [once configured]",
      "Google Fonts — your browser downloads our typefaces from Google, which receives your IP address",
      "Titan — our business mailbox, if you write to hello@thehivesociety.ae",
    ],
  },
  {
    title: "International processing",
    body: [
      "Some of these providers may process information outside the United Arab Emirates. [Owner and counsel to confirm the transfer basis and wording required under applicable UAE law.]",
    ],
  },
  {
    title: "Cookies and browser storage",
    body: [
      "We use one strictly necessary cookie, hive_session, to keep you signed in. It is not readable by page scripts and expires after 14 days or when you sign out.",
      "Your browser remembers whether you chose English or Arabic. [Signup conversion and campaign attribution tracking must be reviewed and disclosed before advertising campaigns begin.]",
    ],
  },
  {
    title: "How long we keep it",
    body: ["We keep information only as long as it is needed:"],
    list: [
      "Your account and profile: until you delete your account",
      "Sign-in sessions: up to 14 days",
      "Email confirmation and password-reset links: valid for 24 hours and 30 minutes respectively, and deleted 7 days after they expire",
      "Email contents: erased once delivered; delivery records are deleted after 30 days",
      "Rate-limit counters: hashed IP and email values held for at most one hour",
      "Server access logs (IP address, time and page requested; never passwords, cookies or form contents): rotated automatically and overwritten after [about 50 MB of logs, typically a few weeks]",
      "Encrypted database backups: [14] days, after which they are deleted. Information you delete can remain in a backup until that backup expires",
      "Security records: [retention period to be decided; currently kept until removed]",
      "The founding community list: until you ask us to remove you",
    ],
  },
  {
    title: "Deleting your account",
    body: [
      "You can delete your account from your profile page after confirming your password. This immediately removes your profile, sign-in sessions, places at gatherings, attendance records, blocks and reports you sent. If a place you held is freed, it is offered to the next person on the waitlist. Reports other members made about you keep their text but are no longer linked to your account.",
    ],
  },
  {
    title: "Your choices and rights",
    body: [
      "You can see and correct most of your details on your profile page, switch optional email updates on or off, and delete your account at any time.",
      "You can also ask us for a copy of the information we hold about you, or ask us to correct or delete it, by writing to hello@thehivesociety.ae. We will reply within [30] days.",
    ],
  },
  {
    title: "How we protect it",
    body: [
      "Connections are encrypted with HTTPS. Passwords are hashed with Argon2id. Repeated sign-in attempts are limited. Administrators must use two-factor authentication. Backups are encrypted before they are stored. Access to member information is limited to our administrators.",
      "No system is perfectly secure. If we become aware of a breach affecting your information we will tell you and the relevant authorities where required.",
    ],
  },
  {
    title: "Age",
    body: ["Membership and gatherings are for women aged 18 and over."],
  },
  {
    title: "Changes and contact",
    body: [
      "If the way we handle your information changes, we will update this page before it does, and tell members by email about significant changes.",
      "Questions or requests: hello@thehivesociety.ae.",
    ],
  },
];

function PrivacyPage() {
  return (
    <div className="pv-page">
      <style>{`
        .pv-page{
          --pv-bg:#FBF8F1; --pv-paper:#FFFDF8; --pv-ink:#22201A; --pv-muted:#6B6558;
          --pv-sage:#3F6E5E; --pv-sage-d:#2F5446; --pv-sage-soft:#E7EEE8;
          --pv-honey:#A4621A; --pv-line:rgba(63,110,94,.16); --pv-line2:rgba(63,110,94,.28);
          background:var(--pv-bg); color:var(--pv-ink);
          font-family:"Manrope","Helvetica Neue",Arial,sans-serif;
          font-size:16px; line-height:1.65; -webkit-font-smoothing:antialiased;
          min-height:100vh;
        }
        .pv-page h1,.pv-page h2{
          font-family:"Cormorant Garamond","Iowan Old Style",Georgia,serif;
          font-weight:500; letter-spacing:-.01em; line-height:1.05; margin:0;
        }
        .pv-page a{color:var(--pv-sage-d)}
        .pv-wrap{max-width:760px; margin:0 auto; padding:0 max(20px,env(safe-area-inset-left,0px))}
        .pv-header{position:sticky; top:0; z-index:10; padding:12px 16px 0}
        .pv-bar{
          display:flex; align-items:center; justify-content:space-between; gap:16px;
          max-width:1180px; margin-inline:auto; padding:10px 20px;
          background:rgba(251,248,241,.86); backdrop-filter:blur(16px) saturate(140%);
          -webkit-backdrop-filter:blur(16px) saturate(140%);
          border:1px solid var(--pv-line); border-radius:999px;
          box-shadow:0 10px 30px -22px rgba(47,84,70,.5);
        }
        .pv-brand{
          font-family:"Cormorant Garamond",Georgia,serif; font-weight:600; font-size:1.2rem;
          letter-spacing:.22em; text-transform:uppercase; text-decoration:none; color:var(--pv-ink);
          white-space:nowrap;
        }
        .pv-back{
          font-size:.88rem; font-weight:600; text-decoration:none; color:var(--pv-sage-d);
          border:1px solid var(--pv-line2); border-radius:999px; padding:8px 16px;
          transition:background .2s;
        }
        .pv-back:hover{background:var(--pv-sage-soft)}
        .pv-hero{padding:clamp(56px,9vw,96px) 0 clamp(28px,4vw,40px); text-align:center}
        .pv-eyebrow{
          display:inline-flex; align-items:center; gap:10px;
          font-size:.74rem; font-weight:600; letter-spacing:.2em; text-transform:uppercase;
          color:var(--pv-sage);
        }
        .pv-eyebrow::before{content:""; width:22px; height:1px; background:var(--pv-sage)}
        .pv-hero h1{font-size:clamp(2.6rem,6vw,4.2rem); margin-top:18px}
        .pv-hero h1 em{color:var(--pv-sage); font-style:italic; font-weight:400}
        .pv-hero p{color:var(--pv-muted); font-size:1.06rem; max-width:52ch; margin:18px auto 0}
        .pv-body{padding-bottom:clamp(64px,9vw,110px); display:flex; flex-direction:column; gap:16px}
        .pv-card{
          background:var(--pv-paper); border:1px solid var(--pv-line); border-radius:28px;
          padding:clamp(24px,4vw,36px);
          box-shadow:0 20px 40px -32px rgba(47,84,70,.45);
        }
        .pv-card h2{font-size:clamp(1.5rem,3vw,2rem); color:var(--pv-sage-d); margin-bottom:12px}
        .pv-card p{color:var(--pv-muted); margin:0 0 10px}
        .pv-card p:last-child{margin-bottom:0}
        .pv-card ul{margin:0 0 10px; padding-left:1.2em; color:var(--pv-muted)}
        .pv-card li{margin-bottom:6px}
        .pv-card li::marker{color:var(--pv-sage)}
        .pv-footer{
          border-top:1px solid var(--pv-line); padding:32px 0 40px;
          text-align:center; color:var(--pv-muted); font-size:.85rem;
        }
        .pv-footer a{color:var(--pv-sage-d); text-decoration:none; font-weight:600}
        .pv-footer a:hover{text-decoration:underline}
      `}</style>

      <header className="pv-header">
        <div className="pv-bar">
          <Link href="/" className="pv-brand" aria-label="The Hive Society, home">
            The Hive Society
          </Link>
          <Link href="/" className="pv-back">
            Back to home
          </Link>
        </div>
      </header>

      <main className="pv-wrap">
        <div className="pv-hero">
          <span className="pv-eyebrow">Your privacy</span>
          <h1>
            Held with <em>care</em>, like everything we do
          </h1>
          <p>
            This page explains, in plain words, what happens to the information you share with The
            Hive Society when you join and take part.
          </p>
          <p role="note" style={{ marginTop: 18, fontWeight: 600 }}>
            Draft for review — not yet approved by The Hive Society or its legal advisers.
            Last updated: October 2026 (draft).
          </p>
        </div>

        <div className="pv-body">
          {sections.map((section) => (
            <section className="pv-card" key={section.title}>
              <h2>{section.title}</h2>
              {section.body.map((paragraph) => (
                <p key={paragraph}>{paragraph}</p>
              ))}
              {section.list && (
                <ul>
                  {section.list.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              )}
            </section>
          ))}
        </div>
      </main>

      <footer className="pv-footer">
        <div className="pv-wrap">
          <p>
            © 2026 The Hive Society ·{" "}
            <a href="mailto:hello@thehivesociety.ae">hello@thehivesociety.ae</a> ·{" "}
            <Link href="/">Back to home</Link>
          </p>
        </div>
      </footer>
    </div>
  );
}


export default PrivacyPage;
