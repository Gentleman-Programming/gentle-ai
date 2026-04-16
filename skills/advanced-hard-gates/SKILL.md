# Advanced Guardrails: Enterprise Pipeline

When you encounter complex, industrial-grade projects where single-agent SDD prompt constraints fail or drift, utilize the **"Hard Gates" execution architecture**.

## The Problem
Single-agent SDD (Explore -> Design -> Spec -> Implement -> Verify) relies heavily on attention retention. Over long sessions, the agent drifts and hallucinates.

## The Solution: Physical Role Isolation (Hard Gates)
Never treat the pipeline as a single conversational entity. Divide the execution physically into four rigid roles:

1. **Planner**: Scopes the problem and creates exact Task Contracts.
2. **Analyst**: Compares <2 architectures against constraints. Does NOT write code. 
3. **Executor**: Translates the active contract into isolated code within the defined boundary. Does NOT touch architecture.
4. **Verifier**: Executes tests. Yields ONLY Pass / Conditional / Fail.

**Hard Rule**: No execution without a Verifier command. The Executor must stop and hand off. Do not allow "interactive debugging" between Executor and Verifier.

## Multi-Layer Verification & RAG Pipeline
For document parsing or rule extraction, use a progressive architecture:
- **L1/L2**: Source routing and structured chunking.
- **L3**: Dynamic Rule Extraction from the knowledge base rather than feeding unstructured docs.
- **L4**: High-variance scoring mechanisms (e.g., executing verification 5 times to ensure variance < 9%).
