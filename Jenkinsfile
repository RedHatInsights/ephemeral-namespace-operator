pipeline {
    agent {label 'insights' }
    environment {
        APP_NAME="ephemeral-namespace-operator"  // name of app-sre "application" folder this component lives in
        COMPONENT_NAME="insights-ephemeral"  // name of app-sre "resourceTemplate" in deploy.yaml for this component
        IMAGE="quay.io/cloudservices/ephemeral-namespace-operator"  // image location on quay

        CICD_URL="https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd"
    }
    stages {
        stage('cicd bootstrap') {
            steps {
                def cicdbootstrap = sh(script: 'curl -s ${CICD_URL}/bootstrap.sh > .cicd_bootstrap.sh')
                echo cicd_bootstrap
            }
        }

        stage('build') {
            steps {
                echo "Hello, World!"
            }
        }

        stage('test') {
            steps {
                echo "Hello, World!"
            }
        }
    }
}
