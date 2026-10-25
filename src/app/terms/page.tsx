import TermsClient from '@/components/screens/TermsClient';
import type { Metadata } from "next";

export const metadata: Metadata = { title: "Terms of Membership", description: "The terms for The Hive Society membership (draft for review)." };

// DRAFT for owner and legal review. Items in [brackets] need an owner/counsel
// decision. Statements describe how the service currently works.
const sections: { title: string; body: string[] }[] = [
  {
    title: "About these terms",
    body: [
      "These terms apply to membership of The Hive Society and to gatherings booked through this website. [Legal entity name, address and governing law to be confirmed with counsel.]",
    ],
  },
  {
    title: "Membership",
    body: [
      "Membership welcomes adults aged 18 and over of all nationalities, women and men, in Abu Dhabi. You need a confirmed email address and approval from our team before booking. Applying does not guarantee approval.",
      "Membership and gatherings are free at launch. If that changes, we will tell members before any charge applies.",
    ],
  },
  {
    title: "Your account",
    body: [
      "Please give accurate details and keep your password private. You are responsible for activity on your account. Tell us at hello@thehivesociety.ae if you think someone else has used it.",
      "You can delete your account at any time from your profile page.",
    ],
  },
  {
    title: "Gatherings, places and the waitlist",
    body: [
      "Gatherings are labelled as mixed-gender or women-only. Check the category before reserving. Each gathering has limited places; when full, the earliest person on the waitlist is promoted automatically if a place opens.",
      "If you can no longer attend, please cancel so the next person can take your place. Places cannot be booked once a gathering has started.",
      "We may change or cancel a gathering; members with a place or on the waitlist are emailed when the date, time or venue changes, or when it is cancelled.",
    ],
  },
  {
    title: "Community conduct",
    body: [
      "Members agree to follow the Community Guidelines: kindness and respect, no selling or promotion, photos only with consent.",
      "You can submit a confidential safety report to our team. Members cannot message one another, and members cannot see other attendees' identities.",
    ],
  },
  {
    title: "Moderation",
    body: [
      "To keep the community safe we may cancel bookings, suspend or end a membership where these terms or the guidelines are broken. [Appeals process to be defined.]",
    ],
  },
  {
    title: "Liability",
    body: [
      "[Wording on responsibility for personal safety at gatherings, third-party venues and partners, and limitation of liability to be provided by counsel.]",
    ],
  },
  {
    title: "Privacy",
    body: ["How we handle your information is explained in our Privacy Policy."],
  },
  {
    title: "Changes and contact",
    body: [
      "We may update these terms; we will tell members by email about significant changes before they apply.",
      "Questions: hello@thehivesociety.ae.",
    ],
  },
];

export default function Terms() { return <TermsClient english={sections} />; }
