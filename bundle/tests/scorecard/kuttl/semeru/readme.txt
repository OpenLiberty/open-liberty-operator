Variation 00: Enable Semeru Cloud Compiler (Check volume, volume mount, env on Semeru deployment, port, affinity on Service. Check the application deployment for arg, volume, volume mount and env as well)
Variation 01: Set the replicas and resources on semeru compiler (Check Deployment to make sure that's updated)
Variation 02: Set enable to false (make sure Deployment and Service no longer exist - negative test)
Variation 03: Enable it again
Variation 04: Change application to StatefulSet (check the application StatefulSet - do the same checks)
Variation 05: Upgrade scenario (image update) - Check Deployment, Service with the new name
Variation 06: Remove .spec.semeruCloudCompiler (In CR set semeruCloudCompiler:) - negative test