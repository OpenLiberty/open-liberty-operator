## Key Concepts / Background
- Open Liberty Operator had a `0.0.1` release in May 2019 where it wrapped around the Open Liberty Helm Chart
  -  Was enough to get the Operator checklist and fit the requirements from a Red Hat Runtimes perspective, but lacked the ability to grow beyond the deployment phase.
- Appsody Operator was re-written in version 0.2.1 to have a core library that works with a `BasicApplication` Go interface
  -  This allowed us to have the `AppsodyApplication` instance which uses that library, as well as any other runtime specific instance, such as `OpenLibertyApplication`.
- We took the Appsody Operator 0.3.0 release as the base library for the re-launch of the Open Liberty Operator, releasing version 0.3.0 to stay in-sync with its upstream library.  

## User stories
- high level scenarios.  Use personas (Todd, Jane, Champ, etc)

## As-is
- what is the current behaviour / experience

## To-be
- what will be the behaviour / experience after the experience (how is it better)
- include sample usage

## Main Feature design

### Relationship with Appsody Operator
Appsody Operator (upstream) --> Open Liberty Operator (downstream)

![Operators](images/downstream_appsody.png)

Appsody Operator Roadmap includes:
*  OpenShift Certificate Manager integration
*  Advanced service binding (resources)
*  Improved rollout support

Open Liberty Operator Roadmap includes everything from Appsody's Roadmap plus:
*  OpenID Connect client configuration injection
*  Transaction peer-recovery
*  Specialized day 2 operations

### Platform exploitation
- Does this feature have any optimization for specific platforms, such as OpenShift?

### Security Considerations
- what are the security aspects of this feature?  are you properly encrypting communication and data-at-rest? which security protocols are being used?

### Limitations
- Document any limitations that this feature has.

### Monitoring
- How does a user monitor this new capability to detect and diagnose problems?

### Performance
- have you done a performance analysis on the impact this feature has?

### Functional Testing
- what test scenarios are suitable to ensure the functionality is correct and does not regress?

### Deprecation / Removal of functionality
- Does this feature deprecate any previous functionality?   
