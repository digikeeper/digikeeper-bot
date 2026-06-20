# Design decision record (ADR)
|Field|Value|
|Date|2025-01-01|
|Status|Under development|
|Author|@gitrus|

## Preface
The Main reason for this ADR is a proposal of an architecture for Telegram bots, to share it with students/community and gather feedback.

Unlike REST APIs, which follow a request-response model, Telegram bots require handling session and state management, user-driven routing, and keyboard interactions.
Developers familiar with REST APIs may struggle with implementing session-aware logic, leading to spaghetti code, implicit state handling, and a lack of abstraction layers.

## Context and Problem Statement
A Telegram bot is a great second "interaction method" for developers after becoming familiar with the REST API.
Why?
- It's easy to create and use. It's popular and has many libraries.
- It combines an RPC-like API for commands WITH Stateful interaction for keyboard interaction
- You can build an application without a frontend, just with backend logic + bot logic. However, you need to consider user behavior (design user journey), not just machine-to-machine interactions.
- Experiences and insights will also enhance the understanding of Rest API as well.

Key Challenges When Transitioning from REST APIs to Bots:
- Session and state management, which may evolve into actor-actor interaction
- Routing that is based not only on URLs but also on the user's state
- Keyboard actions and interaction with user, flows, etc

RestAPI exists to mitigate or even avoid some of these challenges.
It was designed for a request-response approach with the sequence of invocation handled on the client side.
This mindset makes it difficult to implement a session-aware approach. Without related knowledge, it's easy to create a mess in bot code.

COMMON problems in typical bot code:
- Spaghetti code in handlers (locks, global variables, etc)
- Implicit state management, without clear state transitions
- Lack of separation of concerns AND zero abstraction layers

Those problems grow as the complexity of the bot increases.
A well-defined architecture is necessary to ensure testability and maintainability. In other words, it allows adding new features without breaking existing ones.

## Considered Options
After research (including LLM assistance ofcourse), i have choosen the following directions:
- Drawing inspiration from realizations of stateful protocols like TCP, SMTP, statefull SOAP and etc
- Introducing User Session, which is usually avoidabel in REST API, but is necessary in bot interactions

### Alternatives
1. **Problem:** Multi-step interaction
   - **Default approach:** Track current state in each handler and interlocked handlers to handle related user commands.
   - **Option 1**: Finite State Machine (FSM) to handle current state of user separately from handler code.Each user's multistep interaction can be described as a set of states and possible transitions. Transitions may have payloads. Also it may be idempotent and you can replay states and transitions.
     - **Pros:** Provides clear, explicit state definitions as a separate entity. Easy to test and maintain.
     - **Cons:** FSM can be complex to implement and maintain. It may be overkill for simple bots.
   - **Option 2**: DSL language. If scripts of interactions with user may look like a tree: some action can be repetitive and even changeable over time. To change it easier, we can create our own language to describe it in declarative way.
      - **Pros:** Provides a clear, declarative way to describe interactions, can be changed by product/analyst. Easy to change and maintain.
      - **Cons:** Requires custom parsing and execution logic. It's introduces a learning curve for developers.
FSM example:
```
States: [Start, Waiting, Confirm]
Transitions: [
  {From: Start, To: Waiting},
  {From: Waiting, To: Confirm},
]
func toWaiting(payload) {
  // some logic
}
```
DSL example:
```
interaction:
  start:
    message: "Enter your name"
    next: waiting

  waiting:
    message: "Confirm? (yes/no)"
    options:
      yes:
        message: Confirming...
        next: confirm
      no:
        message: Re-enter your name
        next: start

  confirm:
    message: "Thank you!"
    next: done
```
DSL sounds fantastic from an engineering perspective, but it’s difficult to implement and maintain. It’s not popular for bot development, making it hard to find libraries or examples.
Using a DSL for a medium-sized bot seems like overkill. FSM is much more popular but can also be excessive for small bots. A code-based FSM requires additional logic to manage states and transitions.
My main goal is to describe a solution for a bot with several multi-step interactions, so I will choose FSM.

2. **User state between interactions**
   - **Default approach:** Store user state in global variable or in-memory storage. This approach has problems: durability, possible race conditions, not scalable and not testable.
   - **Option 1**: Introduce User Session entity. User session is a user info and state/states storage. It may be fetched from storage and added to each user request by middleware to fetch/save user session by userID. It will be used in handlers to get/set user state.
   - **Option 2**: Session Service as first-class citizen, decoupled from the request lifecycle.. User session will be abstracted properly and be part of buisness logic. Session may be big so UserSession service can aggregate it by request and also save it to storage partialy in diffrent stages in buisness logic.

A separate user session manager abstraction as an actor/service is more scalable, but it requires additional logic to fetch the session and ensure correct state propagation. This approach would be overly complex in the initial stages.
Keeping the user session as part of the request has its own limitations: it can lead to stale state issues if multiple requests for the same user are processed in parallel.
I will choose to manage the user session as part of the request via middleware.
To overcome the stale state issue, there are two approaches:
 - Sticky session – Ensures that a user’s requests are always processed by the same instance.
 - Locking the session – Allows only one active session per user at a time.

3. **Code composition**
  - **Default approach:** Handlers are responsible for business logic and bot logic. Handlers are interlocked and have implicit state management.
  - **Option 1**: Use MVC, where handlers call the Controller-layer to change the model state. The Controller layer is responsible for business logic, including state transitions using defined FSM.
  - **Option 2**: Use Onion/Hexagonal architecture. Handlers are responsible for bot logic and call services for business logic. Services are responsible for business logic and state transitions using defined FSM.

What? The descriptions of the two options are mostly the same. What is the difference between MVC and Onion/Hexagonal?
Even though MVC and Onion/Hexagonal architectures share similarities, the key difference is that in Onion/Hexagonal, the model is not directly related to physical storage—it is merely a representation of the data. Additionally, in MVC, the View is tightly coupled with the bot logic, whereas in Onion/Hexagonal, it is responsible for both the bot logic and user interaction.
The description here is mainly for illustration. In Go, Onion/Hexagonal architecture is significantly more popular. So, I will choose Onion/Hexagonal architecture.

### Decision
Possible balanced approach is:
- Using User Sessions and Middleware to fetch/save user session
- Using Finite State Machine (FSM) to describe session-transaction one FSM per multi-step interaction
- Using Onion/Hexagonal architecture to separate bot logic from business logic

Diagram:
```
        +--------------+
        | User request |
        +--------------+
               |
               v
     +-------------------------+
     | User Session Middleware |
     |                         |
     |  Fetch/save user        |
     |  session by userID      |
     +-------------------------+
               |
               v
 +---------------------------------+
 |             Handler             |
 |                                 |
 |  Bot logic:                     |
 |  Identify session transaction   |
 |  Change user state and/or  sync |
 |  changes in buisness service    |
 +---------------------------------+
               |
               v
  +---------------------------------+
  |         Business Service        |
  |                                 |
  |  Business logic:                |
  |  Process user request           |
  |  Make backand magic             |
  +---------------------------------+
               |
               v
  +---------------------------------+
  |    Repository (Data access)     |
  |                                 |
  |  Data fetch/save logic          |
  |  (DB, cache, etc)               |
  +---------------------------------+
               |
               v
     +-------------------------+
     | User Session Middleware |
     |                         |
     | Save user session       |
     | to storage              |
     +-------------------------+
                |
                v
    +---------------------------+
    | Bot action (aka response) |
    +---------------------------+
```

Those decisions are anchors in terms of agile architecture. It means those decision will be hard to change in future.
Other architecture decisions will be based on this one with an agile approach.

So the most promising "next step" options are:
- A clear method to  create multi-step Wizard-like user interactions. With session-based transactions we can traet multi-step interaction as single transaction.


## Pros and Cons of the Options
This approach likely has trade-offs and pros and cons, but it's definitely better than spaghetti code in handlers.
* I will update this record after implementation and testing.

## Decision Outcome
As an outcome of this ADR I have to find libraries that will support my approach.

For telegram bot updates I need a simple not-opinionated telegram bot library.
The architecture described above is my own opinion, so I need a tiny library that will have enough tools and be easily combined with my approach.
The most promising one is `telego`.

For FSM I need a library that will support FSM with transitions and payloads.
`Statetrooper` is a nice option with a state transitions list and transition can hold payload.

For User Session I need a library that will be compatible with `telego` and will be easy to use.
I will create my own implementation for now because it sounds more like get/set user state by userID.
To resolve the stale state issue I will search and add or copy-paste separate library/pkg.
