# ReCall

ReCall is a video intelligence platform for turning raw video footage into searchable events.

The idea is simple. Video contains a lot of information, but finding one specific event in hours of footage is difficult. Recall processes video, extracts useful information from it, stores that information, and makes it possible to search through the footage later.

Instead of manually watching hours of video, you should be able to ask questions like:

```text
"Show me when someone entered the restricted area."

"What happened before the machine stopped?"

"Find all the times a person fell."

"Show me every vehicle that entered after 10 PM."
```

Recall is being built as a system for doing this locally and at scale.

## Why Recall?

Most video systems are good at recording footage.

The problem starts when you actually need to find something in that footage.

Imagine a camera recording for 24 hours.

If an incident happened at 3:17 PM, someone may have to manually go through the recording to find it.

Basic computer vision does not completely solve this either.

A detector might tell you:

```text
Person detected
Car detected
Person detected
```

But that does not necessarily tell you:

```text
A person entered the restricted area.
The person stayed there for 42 seconds.
The person was not wearing a helmet.
The person then approached a machine.
```

Recall is focused on the layer between raw video and useful information.

```text
Video
  ↓
Detection
  ↓
Tracking
  ↓
Events
  ↓
Search
  ↓
Evidence
```

## The recall problem

There is another problem with processing video: you cannot always afford to process every frame with every model.

Consider a 30 FPS camera recording for one hour.

That is:

```text
30 × 60 × 60 = 108,000 frames
```

Running expensive vision models over every frame can be unnecessary.

A common approach is to sample frames.

For example:

```text
1 frame every 2 seconds
```

This makes processing much cheaper, but it introduces another problem.

An important event can happen between two sampled frames.

```text
Frame A
   │
   │
   │  Person falls
   │
   │
Frame B
```

If the event happens between the frames we inspect, we can miss it completely.

This is a recall problem.

Recall is designed around the idea that **missing an important event can be worse than spending more compute to verify it.**

The system can use a multi-stage approach:

```text
                 Video
                   │
                   ▼
            Cheap analysis
                   │
                   ▼
             Something
              interesting?
              /        \
            No          Yes
            │            │
            ▼            ▼
         Continue    More analysis
                         │
                         ▼
                    Higher FPS
                         │
                         ▼
                     Tracking
                         │
                         ▼
                  Event detection
                         │
                         ▼
                     Confirmed
                       event
```

The goal is to use cheap processing for normal footage and spend more compute when there is evidence that something important may be happening.

This should allow Recall to improve event recall without blindly running expensive models over every frame.

## What Recall is trying to solve

Recall is not intended to be just a video player with an AI search box.

The long-term goal is to build a system that converts video into structured events.

For example:

```text
Video
  │
  ├── People
  ├── Vehicles
  ├── Objects
  ├── Locations
  ├── Actions
  ├── Speech
  ├── Text
  └── Events
```

Those events can then be searched and connected through time.

For example:

```text
14:31:04
Worker approaches machine

14:31:18
Worker opens machine panel

14:31:32
Warning light appears

14:31:41
Machine stops
```

This allows questions that depend on context rather than a single frame.

## Planned architecture

The architecture is being developed around separate processing services and workers.

At a high level:

```text
                         Video
                           │
                           ▼
                    Video Ingestion
                           │
                           ▼
                    Frame Processing
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
          Detection      Tracking      Audio
              │            │            │
              └────────────┼────────────┘
                           ▼
                     Event Detection
                           │
                           ▼
                     Event Storage
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
         Vector Search  Event Search  Metadata
              │            │            │
              └────────────┼────────────┘
                           ▼
                       Query API
                           │
                           ▼
                    Search Results
                           │
                           ▼
                      Video Clips
```

The exact architecture will evolve as the project develops.

## Current state

Recall is currently under development.

The repository is being built in Go and already has the initial service and worker structure in place.

Current repository structure:

```text
recall/
├── migrations/
├── pkg/
│   └── database/
├── services/
├── workers/
│   └── detector/
├── docker-compose.yml
├── Makefile
├── go.mod
└── go.sum
```

The detector is being developed as a separate worker so that video processing can be scaled independently from the rest of the platform.

## Where this can go

The initial focus is video processing and event detection.

Over time, Recall can support several types of queries.

### Semantic search

```text
"person falling near the entrance"
```

### Object search

```text
"red car"
```

### Event search

```text
"person entering restricted area"
```

### Temporal search

```text
"what happened before the machine stopped?"
```

### Combined search

```text
"Find workers without helmets
who entered Zone B
and stayed for more than 30 seconds."
```

The important part is that the result should not just be text.

Recall should return the evidence from the original footage:

```text
Event
  ↓
Timestamp
  ↓
Relevant frames
  ↓
Video clip
```

## Design goals

### Local first

Video footage can contain sensitive information.

Recall is being designed so that the core processing can run on infrastructure controlled by the user instead of requiring every video to be uploaded to a third-party AI service.

### Efficient processing

Video is expensive to process.

Recall should avoid doing expensive inference where it is unnecessary and instead use multiple stages of processing.

### High recall

Missing an important event is a serious problem for systems that are meant to analyze footage.

The system should therefore be designed to detect possible events cheaply first and then spend additional compute verifying them.

### Searchable events

Raw detections are not enough.

The useful output is a structured representation of what happened and when it happened.

### Evidence first

Every important result should be traceable back to the original video.

If Recall says something happened, the user should be able to see the footage that supports the result.

## Why build this?

Video is becoming easier to capture, but the amount of footage generated by cameras is growing much faster than the ability of people to manually inspect it.

There is a lot of useful information trapped inside that footage.

Recall is an attempt to build the infrastructure needed to turn that footage into something that can be searched and understood.

The project is also an experiment in combining:

* computer vision
* video processing
* machine learning
* information retrieval
* temporal reasoning
* distributed processing
* backend systems

## Tech stack

The stack is still evolving, but the current project is primarily built around:

* Go
* Docker
* PostgreSQL
* Computer vision models
* Video processing with FFmpeg
* Background workers

Additional components will be introduced as they become necessary.

## Status

Recall is an active work in progress.

The current priority is getting the core video processing pipeline working reliably before adding more advanced search and reasoning capabilities.

