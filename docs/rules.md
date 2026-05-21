# PulseOps AI — Hackathon Rules & Constraints

> Reference document for the Google Cloud Rapid Agent Hackathon.
> All project decisions must comply with the rules defined here.
> Source: https://rapid-agent.devpost.com

---

## Contest Overview

| Field | Detail |
|---|---|
| Contest Name | Google Cloud Rapid Agent Hackathon — Google Cloud Partnerships |
| Sponsor | Google LLC |
| Administrator | Devpost, Inc. |
| Contest Site | https://rapid-agent.devpost.com |
| Track Selected | **Elastic** |

---

## Contest Period

| Milestone | Date / Time |
|---|---|
| Contest Opens | May 5, 2026 — 12:00 PM PT |
| **Submission Deadline** | **June 11, 2026 — 2:00 PM PT** |
| Judging Period | June 22, 2026 – July 6, 2026 |
| Winners Announced | On or about July 7, 2026 |

> All submissions received after the deadline will be disqualified.

---

## Prize — Elastic Track

| Place | Prize |
|---|---|
| 🥇 First Place | $5,000 USD + social media promotion opportunity |
| 🥈 Second Place | $3,000 USD |
| 🥉 Third Place | $2,000 USD |

---

## What Must Be Built

Build a **functional agent** that:

- Is powered by **Gemini** and **Google Cloud Agent Builder**
- Integrates the **Elastic MCP server** meaningfully
- Solves a real-world challenge
- Performs actual tasks beyond answering questions (tool use, multi-step reasoning, autonomous actions)

### PulseOps AI satisfies this by:

- Using **Gemini** for root-cause analysis and incident summarization
- Using **Google Cloud Agent Builder** as the primary orchestration and workflow layer
- Using **Elastic MCP** to query logs, retrieve telemetry, and correlate incidents
- Executing real remediation actions (restart services, flush DNS)
- Operating a full detect → investigate → remediate → validate → summarize workflow

---

## Required Technology Stack

| Layer | Required | PulseOps Implementation |
|---|---|---|
| AI Reasoning | Gemini (Google Cloud) | Gemini via Agent Builder |
| Orchestration | Google Cloud Agent Builder | Agent Builder workflows |
| Partner Integration | Elastic MCP Server | Elastic MCP for log/telemetry queries |
| Platform | Web, Android, or iOS | Web (React dashboard) |

---

## Prohibited Technologies

The following are explicitly NOT permitted inside the final submitted product:

- ❌ OpenAI APIs
- ❌ Anthropic / Claude APIs
- ❌ Grok / xAI APIs
- ❌ Any AI tools that compete directly with Google Cloud AI services
- ❌ Any cloud platform services that directly compete with Google Cloud
- ❌ Any partner-competing services within the Elastic track

> Rule reference: "The use of other services that directly compete with Google Cloud (for cloud platform capabilities) or with the Partner whose track you've selected is not permitted."

---

## Project Requirements

### Must be newly created

- The project must be **newly created during the Contest Period** (May 5 – June 11, 2026)
- Must not be a modification or extension of any existing prior work
- Must be the entrant's **original creation**

### Must be functional

- The project must be **capable of being successfully installed and run**
- Must function as depicted in the submitted demo video
- Must function as described in the text description

### Must run on a supported platform

- Web ✅ (selected)
- Android (not selected for this project)
- iOS (not selected for this project)

### Third-party integrations

- Any third-party SDK, API, or data used must be **properly licensed**
- Entrant is responsible for compliance with all third-party tool terms

---

## What Must Be Submitted

The following are all **required** for a valid submission:

| Submission Item | Requirement |
|---|---|
| Hosted Project URL | Live, accessible, functional deployment |
| Code Repository URL | Public GitHub repo, open-source license visible in About section |
| Text Description | Summary of features, technologies used, learnings |
| Demo Video | Max 3 minutes, publicly visible on YouTube or Vimeo, in English |

### Demo Video Rules

- Must show the project **functioning** on the target platform
- Must be **3 minutes or less** (only first 3 minutes evaluated if longer)
- Must be hosted publicly on **YouTube or Vimeo**
- Must be in **English** or include English subtitles
- Must not contain third-party advertising, logos, or trademarks
- Must not contain offensive, inappropriate, or legally problematic content
- Must be an **original, unpublished work**

### Repository Rules

- Must be **public**
- Must include a **complete open-source license file** (OSI-approved)
- License must be **detectable and visible** in the GitHub About section
- Must contain all source code, assets, and setup instructions

---

## Judging Criteria

Submissions are evaluated in two stages:

### Stage One — Pass/Fail Baseline

Judges verify:
- All required submission items are present
- The project reasonably addresses the challenge
- Google Cloud and Partner (Elastic) products are meaningfully applied

### Stage Two — Scored Evaluation (Equal Weight)

| Criterion | What Judges Look For |
|---|---|
| **Technological Implementation** | Quality of interaction with Google Cloud and Elastic services |
| **Design** | Quality of user experience and interface design |
| **Potential Impact** | How significant the impact could be on the target community |
| **Quality of Idea** | Creativity and uniqueness of the project concept |

> Ties are broken by comparing scores in the order listed above.

---

## Eligibility

- Must be above the age of majority in country of residence
- Must not be a resident of embargoed or excluded countries (including China, Russia, Iran, North Korea, and others listed in the official rules)
- Must not be an employee, intern, or contractor of Google, Elastic, Devpost, or affiliated organizations
- Team size: maximum **4 individuals**
- Each team member must be added to the Devpost project

---

## Intellectual Property

- The entrant **retains ownership** of their Submission
- Google is granted a license to use the submitted video for evaluation and promotion
- The project source code must be released under an **OSI-approved open-source license**
- Entrant warrants the Submission is their original work and does not infringe third-party rights

---

## Key Constraints for PulseOps AI Development

### Must do

- [ ] Use Gemini as the AI reasoning engine
- [ ] Use Google Cloud Agent Builder as the orchestration layer
- [ ] Integrate Elastic MCP Server meaningfully (not cosmetically)
- [ ] Deploy a live hosted version before June 11, 2026
- [ ] Publish the repo publicly with an OSI-approved license in the About section
- [ ] Record and publish a demo video (max 3 minutes) on YouTube or Vimeo
- [ ] Submit via Devpost before 2:00 PM PT on June 11, 2026

### Must not do

- [ ] Use any competing AI APIs (OpenAI, Claude, Grok, etc.)
- [ ] Repurpose or extend any pre-existing project
- [ ] Submit after the deadline
- [ ] Leave the repo private or without a license

---

## Submission Checklist

- [ ] Hosted project URL is live and functional
- [ ] GitHub repo is public with OSI license visible in About section
- [ ] Demo video is uploaded to YouTube or Vimeo (max 3 min, English)
- [ ] Text description is written (features, tech stack, learnings)
- [ ] Devpost submission form is completed
- [ ] Track selected: **Elastic**
- [ ] All team members added to Devpost project

---

*Last updated: May 2026*
*Source: https://rapid-agent.devpost.com/rules*