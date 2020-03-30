# Troubleshooting

Here are some basic troubleshooting methods to check if the operator is running fine:

* Run the following and check if the output is similar to the following:

  ```console
  $ oc get pods -l name=open-liberty-operator

  NAME                                     READY   STATUS    RESTARTS   AGE
  open-liberty-operator-5c4548d98f-xgqtg   1/1     Running   0          2m29s
  ```

* Check the operators events:

  ```console
  $ oc describe pod open-liberty-operator-5c4548d98f-xgqtg
  ```

* Check the operator logs:

  ```console
  $ oc logs open-liberty-operator-5c4548d98f-xgqtg
  ```

If the operator is running fine, check the status of the `OpenLibertyApplication` Custom Resource (CR) instance. 

Note that the following commands use `olapp`, which is the short name for `OpenLibertyApplication`:

* Check the CR status:

  ```console
  $ oc get olapp my-liberty-app -o wide

  NAME                      IMAGE                                                     EXPOSED   RECONCILED   REASON    MESSAGE   AGE
  my-liberty-app            quay.io/my-repo/my-app:1.0                                false     True                             1h
  ```

* Check the CR effective fields:

  ```console
  $ oc get olapp my-liberty-app -o yaml
  ```

  Ensure that the effective CR values are what you wanted.

* Check the `status` section of the CR. If the CR was successfully reconciled, the output should look like the following:

  ```console
  $ oc get olapp my-liberty-app -o yaml

  apiVersion: openliberty.io/v1beta1
  kind: OpenLibertyApplication
  ...
  status:
    conditions:
    - lastUpdateTime: "2020-01-08T22:06:50Z"
      status: "True"
      type: Reconciled
  ```

* Check the CR events:

  ```console
  $ oc describe olapp my-liberty-app
  ```