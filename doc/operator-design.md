## Key Concepts / Background
- introduce here any background concepts, relevant existing frameworks, acronyms, etc.  

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
