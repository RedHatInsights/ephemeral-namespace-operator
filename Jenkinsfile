pipeline {
    agent { label 'insights' }
    options {
        timestamps()
    }
    environment {
        TEARDOWN_RAN=0
        GO_TOOLSET_IMAGE='registry.access.redhat.com/ubi9/go-toolset:1.18.9'
    }

    stages {
        stage('Run Unit Tests') {
            steps {
                sh '''
                    . ./ci/helpers.sh
                    . ./ci/unit_tests.sh
                '''
            }
        }
    }

    post { 
        always {
            archiveArtifacts artifacts: 'artifacts/**/*', fingerprint: true
            junit skipPublishingChecks: true, testResults: 'artifacts/junit-eno.xml'
        }
    }
}
