'use client';
import Link from "next/link";

const sections = [
  {
    title: "What we collect",
    body: ["When you fill in the “Join the founding community” form, we collect:"],
    list: [
      "Your name",
      "Your email address",
      "Your phone number, if you choose to share it",
      "Your note about what draws you to The Hive, if you choose to write one",
      "The language you were browsing in and the date you sent the form",
    ],
  },
  {
    title: "How we use it",
    body: ["We use these details only to run the founding community:"],
    list: [
      "To read your application personally — every request is read by a person, not filtered by software",
      "To contact you about the founding community, as you asked us to when you ticked the consent box",
      "To invite you first when our first season of gatherings opens",
    ],
  },
  {
    title: "What we never do",
    body: [
      "We never sell your information, and we never share it with advertisers or data brokers.",
      "We use Google Fonts to display our typefaces, Lovable Cloud to host this website and hold signup submissions, and Lovable AI to answer optional questions and provide interest recommendations. Like most websites, these services may briefly record technical details such as your IP address and browser type to deliver the page and keep it secure. These providers may process information outside the United Arab Emirates.",
      "Your browser remembers only whether you chose English or Arabic, so the site opens in your language next time. We don't use advertising or tracking cookies.",
    ],
  },
  {
    title: "Optional AI questions and recommendations",
    body: [
      "When you use Ask The Hive, your question, the current conversation and your selected language are sent through the Lovable AI Gateway to an AI model so it can answer questions and follow-ups about The Hive Society. When you request personal suggestions, your interests and language are sent to recommend relevant gathering categories and next steps. We do not send your signup name, email or phone number with either request.",
      "The Hive website does not save questions, interest requests or AI responses to the community database or browser storage. They remain only in your current page until you reload, leave, change mode or language, or start a fresh conversation. Lovable processes requests to operate the AI service; its logging and privacy settings govern service-side handling.",
      "Please do not include contact details or sensitive information. You can skip AI entirely and use the founding community form. Suggestions are not bookings, membership decisions or guarantees; applications are still read personally.",
      "If you choose “Continue to the founding community” after interest recommendations, your original interests are copied into the optional signup note. You can edit or remove them before sending the form. Only then are those interests saved with your application. Chat questions and answers are not copied into the signup form.",
    ],
  },
  {
    title: "Members' area",
    body: [
      "When you create a members' account we store your first name, email, optional phone number, area of Abu Dhabi, interests, an optional photo, your password (securely encrypted — we can never read it), the date you agreed to our community guidelines and this policy, your membership status and the gatherings you've reserved or joined the waitlist for.",
      "Other members can never see your email, phone number or photo. Only our founder, as the site's admin, can see member details, to review applications and to know who is coming to each gathering.",
      "A new account also triggers an email alert to hello@thehivesociety.ae with your first name, area and interests, plus a link to the private admin page. Lovable manages email delivery; copies received in the admin mailbox are separate from your account and can be removed on request. Alerts begin once the sender domain is verified.",
      "You can update your details at any time on your profile page, and delete your account there. Deleting removes your profile, photo and places at gatherings straight away.",
    ],
  },
  {
    title: "How long we keep it",
    body: [
      "We keep your details while the founding community list is active, so we can reach you when the season opens. You can ask us to remove them at any time and we will.",
    ],
  },
  {
    title: "Your choices",
    body: [
      "You can ask to see the details we hold about you, correct them, or have them deleted. You can also withdraw your consent to be contacted at any time.",
      "Just write to us and we'll take care of it personally.",
    ],
  },
  {
    title: "Talk to us",
    body: [
      "The Hive Society is based in Abu Dhabi, United Arab Emirates. For anything about your information, email hello@thehivesociety.ae.",
      "This page was last updated in October 2026. If the way we handle your information changes, we'll update this page before it does.",
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
            Hive Society through the founding community signup form.
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
