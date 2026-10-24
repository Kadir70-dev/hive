import {
  Body,
  Button,
  Container,
  Head,
  Heading,
  Html,
  Preview,
  Text,
} from "@react-email/components";
import type { TemplateEntry } from "./registry";

interface Props {
  firstName?: string;
  area?: string;
  interests?: string[];
}

const brand = {
  paper: "#ffffff",
  sage: "#3F6E5E",
  ink: "#1F1A14",
  muted: "#6E6253",
};

function NewMemberAlert({ firstName, area, interests }: Props) {
  return (
    <Html lang="en" dir="ltr">
      <Head />
      <Preview>A new member request is ready for your personal review.</Preview>
      <Body
        style={{ backgroundColor: brand.paper, fontFamily: "Arial, sans-serif", color: brand.ink }}
      >
        <Container style={{ maxWidth: "560px", padding: "32px 24px" }}>
          <Text style={{ color: brand.sage, fontSize: "14px" }}>THE HIVE SOCIETY</Text>
          <Heading style={{ fontFamily: "Georgia, serif", fontWeight: 400, fontSize: "30px" }}>
            A new member request
          </Heading>
          <Text>
            {firstName || "A prospective member"} has created an account and is ready for your
            personal review.
          </Text>
          {area && <Text style={{ color: brand.muted }}>Area: {area}</Text>}
          {interests?.length ? (
            <Text style={{ color: brand.muted }}>Interests: {interests.join(", ")}</Text>
          ) : null}
          <Text>
            You can review their details and membership status privately in your admin page.
          </Text>
          <Button
            href="https://thehivesociety.ae/admin"
            style={{
              backgroundColor: brand.sage,
              color: brand.paper,
              borderRadius: "6px",
              padding: "13px 20px",
              fontSize: "14px",
            }}
          >
            Review member request
          </Button>
        </Container>
      </Body>
    </Html>
  );
}

export const template = {
  component: NewMemberAlert,
  subject: "New member request · The Hive Society",
  displayName: "New member alert",
  to: "hello@thehivesociety.ae",
  previewData: { firstName: "A new member", area: "Abu Dhabi", interests: ["Majlis", "Wellness"] },
} satisfies TemplateEntry;
