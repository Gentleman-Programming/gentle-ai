# The story of fixing RDD

> How two sleepless days, an entire community and three monthly resets turned into a release. For the technical detail, see [organic-rdd.md](organic-rdd.md).

## What RDD is, in one sentence

When you change something important, someone reviews it before it ships. That is all.

The hard part is not the idea, it is that it **must not get in the way**. A system that forces ceremony to change a comma gets uninstalled in three days. One that says nothing when you touch authentication is worth nothing.

## How it works now

You change something. The tool looks at **what** you changed, not how much.

- **Edited a README** → it asks nothing. Zero ceremony.
- **Wrote a thousand lines of documentation** → still nothing. Size does not matter.
- **Touched two lines of login code** → four reviewers.

And if you want none of it:

```
gentle-ai review mode disable
```

Done. It is off. **Not "off but still in your way"** — off. Do whatever you want, and if you turn it back on it tells you it is going to re-validate whatever was never reviewed.

---

## The part nobody tells

### It started badly, and not for the reason you would guess

The first version of this was built with Codex GPT 5.6 in ultra mode, and what came out was enormous. It was also not what needed doing.

The easy version of that story is that someone let a model loose and it went wrong. That is not what happened, and the real version is the one worth telling.

**The audit document was mine.** An agent wrote it, and I was behind every line: the facts it stood on, the reasoning, the architecture it described. Every decision that mattered was made by a person who had sat down and thought about it.

Which is exactly how I tell everyone to work. You direct, the agent executes, the human leads. I was not skipping that step. I was doing it.

And the agent built something else anyway.

It read that audit, saw it mention enterprise-level requirements, and **inferred** that HTTP support, remote execution and a whole infrastructure for large teams were needed. Nobody had asked for any of it. It deduced a need out of a document that was about something else, and then built all of it. Complete. Coherent. Well made.

Then all of it had to come out. Removing something large and well-built is harder than removing something broken, because **it looks like it works**.

That consumed **three monthly Codex resets**.

### Why it happened

Not because the thinking was skipped. Because nothing was holding the agent to it.

I was doing it the way the big companies do it: assemble the context, hand it to the model, trust what comes back. What I was not doing was using my own tool.

That is the whole argument for this thing, and I paid three resets to learn it concretely. **Good human decisions do not survive contact with an agent unless something enforces them.** A document that states the architecture is a suggestion, and an agent under pressure to be helpful will read a suggestion as a starting point. Phases with explicit contracts, one writer per lane, verify before asserting, and a failing existing test that stops the work are not suggestions.

The architecture was never the problem. The absence of anything making it binding was.

### The second run

Same model class. Same person. Same decisions. What changed was that the decisions were now enforced by **phases with explicit contracts, one writer per lane, verify before asserting, and the rule that a failing existing test is never edited — you stop and report.**

That last rule alone caught **nine wrong premises**. Nine times an agent was about to fix something, an old test went red, and it turned out the test was right and the diagnosis was not.

After two days of working flat out, I am still at **66% of the weekly limit**. The difference was not the model. It was the method.

---

## What the community found

This is the part I like most.

I shipped a pre-release and people broke it. In the good way.

**@Wladimirfn, @Denver2828, @MarsSall and @Freedom2828** reported the same failure from four angles. It looked like a Windows bug. It was not: it happened when the reviewed commit had already been published. Denver2828 reached the same diagnosis independently, building the branch with print statements, and **his patch was identical to mine, line for line**.

**@ElCaaarnal** typed a flag by hand and hit something I had announced as fixed. He was right: I had fixed the tool so it stopped *printing* the broken form, not the parser so it would accept it. **The changelog overclaimed and he lost time to it.**

**@ardelperal** reported a command exiting successfully when it should have failed. I investigated: it was a measurement trap. In bash, `$?` gives the status of the *last command in the pipeline*, not the binary. His report was not a bug, but it documented a trap that would have cost the next person an afternoon.

**@Blue-XL** found that a deliberately forged authorization was accepted and stored in the audit record as though genuine. Worse than having no field: an absent authorization is honestly absent, **a wrong one lies**.

**@AlbertGC13** found two things on Windows with a rigour worth copying: he separated explicitly what he had tested from what he had only read in the code, and **stated what he was not claiming**. He found a Git permissions refusal being turned into advice that could not possibly be followed.

**@edwinsaavedran** showed that four macOS defects had escaped because CI never runs on Darwin, and built the case with a link to each one.

**@Matere413** found that a reviewer result my own agents produce is rejected by my own admission, because two of my documents disagree about the required shape.

**@Andiveli** found the kill switch's hardest case: reviews off, but old approved receipts still lying around in the store. It used to fail with `authority_corrupted`. He came back on the next refresh, reran the exact scenario, and confirmed the fix reports the state without faking an approval.

**@decode2** did not report a bug, he reported a **deadlock**. A corrected candidate that passes verification and holds an approved review still could not finish its remediation, because finishing needs a distinct successor, creating a successor needs an invalidated predecessor, and invalidating is refused for a healthy approved one. Three rules, each defensible alone, forming a closed ring. He reproduced it against the current head rather than an old tag and wrote out the cycle edge by edge.

**@danielxxomg** went looking for what a well-behaved tester would not do. He killed `review start` mid-flight with `kill -9` and then checked whether the store came back clean, whether the lock leaked, whether anything was corrupted. It recovered. That case is nowhere in my own corpus.

**@AndySabina** ran the guide twice across refreshes on WSL2, published the SHA-256 of the exact binary tested both times, and, when the second round found nothing, **said so and opened nothing**. A clean report that stays quiet is worth as much as a defect and is much rarer.

**@frankirova** passed all eight flows and then did the thing almost nobody does: he stated what he had **not** covered. He noticed the asset he downloaded was built from a tag two commits behind the branch, named both commits, and reasoned about whether they touched the paths he was testing. A result is only as good as its stated scope.

**@ftorga** re-validated behaviour-first against the tag and the live head, found two gaps still reproducible, and linked each one to the specific comment where it had been validated before, so nothing had to be taken on trust.

**@MarcosArispe, @dnlrsls, @GinoL221, @orlo-dragomir, @lu149e, @salema97, @diegofercho21323, @blickcbot, @Deco** and several more kept testing refresh after refresh.

None of those findings came from an internal audit. **They came from people using the tool.**

---

## The audits: the ones that worked and the ones that did not

### The ones that worked

The mechanical ones. The ones derived from the code rather than from a list someone has to remember to update.

One walks the syntax tree looking for error messages that name a command, and checks that the command and its flags **actually exist**. It found messages pointing at things that were not there.

Another rejects new functions nobody calls. When I removed the Codex cleanup, it told me **fifteen functions** had gone dead — an entire parser that existed only for that. I deleted them following that evidence.

That guard was eight hours old when it found its first real defect.

### The ones that did not

The ones that verified something was **emitted**, never that it was usable.

The perfect case: there was a message telling you "to get out of this, run this command". There were tests. They verified the message was emitted, with its exact text. All green.

**Nobody had ever run the command the message named.**

When I ran it, it did not work. I had been sending people into a dead end, with green test coverage, for months.

That is where the rule governing everything else came from:

> **A message may name a command only if running that command resolves the block.**

Naming a dead end is worse than naming nothing.

---

## The benchmark

At some point I stopped arguing about whether it was better and measured it.

The tool counts how often you get stuck and, above all, **how** you get stuck:

- **In band** — it stops you and tells you what to run
- **Out of band** — it stops you and tells you nothing
- **Dead end** — it stops you and there is nothing you can do

It does not measure speed. Speed depends on the provider and the day; friction is yours.

And it ships. It is in the repository, it drives a real binary rather than a mock, and you can point it at your own build:

```
cd bench && go run . run --binary $(command -v gentle-ai)
```

That is deliberate. A number I publish and you cannot reproduce is a claim about my honesty. A number you can run yourself is evidence.

First measurement: **six blocks, every one of them out of band.**

Then the corpus itself was the problem. Fourteen journeys, all drawn from the testing guide, so it measured the paths a tester already walks and nothing else. Twenty-two more went where those never did: bare repositories, linked worktrees, a merge left half-finished, a file with no bytes in it, the switch flipped in the middle of a review.

Latest, over all thirty-six: **zero dead ends. Fifteen blocks in band, three out of band, two declared correct by design.**

The block total did not move. That is the point, and it took a while to see: the tool does not stop you less than it did. It stops you exactly as often, and now it tells you how to get out.

Two things that measurement taught me about measuring. The first run after a round of fixes produced numbers identical to the run before it, which is what made me look: the build had failed silently and I had measured the old binary. The second was worse. The analyzer was counting the kill switch **working** as friction the tool caused, because a gate that hands delivery back to ordinary policy still reports `allowed: false`. It says so in the same breath, in its own words, and nothing was stopped. An instrument that flatters you is useless; one that convicts you of the wrong crime is worse.

A tester said it better than my own tool: *"it communicates the state correctly, but proposes no continuation command"*.

---

## The loop

Once the benchmark existed, the obvious move was to stop reading it and start running it. Measure, fix what it found, rebuild, measure again. Keep going until the number stops moving.

That is a small idea with one sharp edge: **the loop only works if you are allowed to find that the instrument is wrong.** A loop that can only ever fix the product will happily converge on a lie.

Round one was tidy. Two blocks named a maintainer when the operator could have cleared them alone, and in both cases the correct reason was already sitting in the JSON — computed, published for machines, and thrown away on the line a human reads. Two others turned out to be refusing correctly, which was worth the same as fixing them, because now it is written down why.

Then the corpus grew to thirty-six and round two found the one I would not have found on my own.

**The kill switch did not stop anything from being written.** Turn reviews off, and `review start` refuses, correctly. Run `review finalize` and it returns success, state approved, terminal receipt on disk. A review had been approved with reviews switched off.

The cause was almost funny. The authorization function was correct. It had two modes, one for starting and one for advancing, and the advancing one was documented, in the source, as *"Disabled mode rejects it"*. It was called from nowhere. One production caller, always passing the other mode. **The guard was written, reasoned about, commented, and never wired up** — which is exactly why a unit test on it passed happily the whole time.

And it mattered beyond tidiness, because the promise is that turning reviews back on re-validates whatever was never reviewed. A receipt minted while the switch was off survives being turned back on, and nothing re-validates it.

Round three, I broke my own measurement. I built the binary with the wrong package path, the build failed, the `&&` did not stop the run that followed, and the benchmark measured the previous binary. The tell was that the totals came back **byte-identical** to the run before. Numbers that do not move after real changes are not a comfort, they are a symptom.

Round four found the same defect for the third time on this branch. The specific reason computed, published to the machine envelope, discarded on the human path. Three separate places, months apart. That stopped being a bug the second time; the third time it is a shape, and it is now written down as one.

That round also caught something more embarrassing than a bug: an escalated review told the operator to `run review.status`. It looks like a command. It is the internal routing name, and typing it does nothing. Worse, when I first fixed it I was about to translate it into the real command — and the agent doing the work proved that the real command **also** resolves nothing there. It describes the state; it does not move it. Naming a command that runs and does not help is more expensive than naming nothing, because now the person trusts it.

The loop is still running. That is not a failure to converge; it is what a loop looks like while it is honest.

---

## The mistakes I made

Because if this is going to be honest, it goes in whole.

**I wrote guide steps without running them.** Three times. A tester followed them, they did not work, and reported the failure. A new rule came out of that: before naming a continuation, execute it.

**I turned a finding into a documentation patch.** Three different testers could not complete a flow. Instead of taking that as the data it was, I wrote the recipe into the guide. The maintainer called it out: doing that **destroyed the measurement** and hid the defect. I reverted it. The real defect was that the tool had a command emitting exactly what was needed, and no path led to it.

**I staged a file without reading its diff** while an agent was writing in it. I swept up 154 lines of someone else's half-finished work and pushed a branch that did not compile. I have ratchets, guards, and tests that demand commands work. **None of that protects you from a hasty `git add`.**

**I chased a defect that was my own measurement error.** I wrote a command's output inside the repository I was measuring, which added a file, changed the state, and the system correctly refused. I lost an hour. But something good came out: that refusal explained nothing either, so I fixed it, and it is now documented as a trap in the guide.

---

## Where it landed

The four macOS defects: closed and verified on real hardware, not on a synthetic profile.

Windows updates itself for the first time.

Codex used to start up broken after syncing, and now it **does not touch its configuration file at all** — verified with the same inode number before and after, meaning it is not opened for writing at all, not merely that the same bytes get written back.

The kill switch is a kill switch.

And things remain open, written down in the technical document, because an honest list of what is missing is worth more than a release claiming everything is done.

---

## What I learned

**Good human decisions do not survive contact with an agent unless something enforces them.** I directed the audit, the reasoning and the architecture, and the agent still built something nobody asked for. Directing well is necessary and it is not sufficient. That gap is the entire reason this tool exists, and I paid three monthly resets to find out I needed my own.

**A test that verifies something was emitted does not verify it is usable.** That distinction explains nearly every defect in this branch.

**A guard nobody calls is not a guard.** The kill switch had one written, documented, and reasoned about, refusing exactly what it should have refused, wired to nothing. Every unit test of it passed. Correct code that is never reached is indistinguishable from code that was never written, and it is more dangerous, because it reads like coverage.

**Point the instrument at itself first.** The benchmark spent a round counting the kill switch working as friction the tool caused, and a round measuring a binary I had failed to build. Both times the number looked plausible. A measurement you cannot audit is an opinion with decimals.

**Dead code that is still documented is a lie.** There was a function that installed dependencies. Nothing called it. The docs said the tool installed dependencies. A Linux user read that and expected it to work.

**Over-engineering is harder to remove than a bug.** A bug is visible. An entire architecture nobody asked for, well built and coherent, defends itself.

**The community finds what audits do not.** The four most valuable reports of these days came from people using the tool on their machine, with their repository, with their odd configuration. No internal audit would have found them, because an audit looks for what you already know to look for.

**And the rule that ran over everything else:** if you tell someone what to do, make sure it works.
